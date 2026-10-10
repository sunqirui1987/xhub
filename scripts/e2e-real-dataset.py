#!/usr/bin/env python3
"""自动启动私有网关、真实供应商观察器和浏览器，运行 docs 数据集并清理。"""
import argparse
from contextlib import contextmanager
import fcntl
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
# 脚本入口的搜索路径默认只有 scripts；加载数据集前补齐同目录辅助模块，
# 使 make e2e 无需额外设置 PYTHONPATH。
sys.path.insert(0, str(ROOT / "e2e"))
spec = importlib.util.spec_from_file_location("real_dataset", ROOT / "e2e/real_dataset.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

SHARED_DATABASE_URL = "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable"
SHARED_REDIS_URL = "redis://127.0.0.1:6379/1"
SHARED_DIRECTORY = ROOT / ".e2e/real-acceptance-current"
SHARED_LOCK = ROOT / ".e2e/real-acceptance-shared.lock"


class RunLogger:
    """把脱敏后的过程日志同步写到终端和运行目录。"""

    def __init__(self, path):
        """用途：初始化本轮日志；参数为日志路径，返回实例；覆盖同阶段旧日志并允许多线程供应商回调安全写入。"""
        self.path, self.lock = Path(path), threading.Lock()
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.path.write_text("")

    def log(self, message):
        """用途：输出带时间的单行日志；参数为脱敏正文，无返回值；多行会拆分，立即刷新终端和文件。"""
        lines = str(message).splitlines() or [""]
        with self.lock:
            with self.path.open("a") as stream:
                for line in lines:
                    rendered = time.strftime("[%Y-%m-%d %H:%M:%S] ") + line
                    print(rendered, flush=True)
                    stream.write(rendered + "\n")
                stream.flush()


def command(args, **kwargs):
    """用途：执行不含秘密的子命令；参数为 argv 和运行选项，返回完成结果；失败抛异常，调用方 finally 清理进程。"""
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)


def command_stream(args, logger, **kwargs):
    """用途：运行长命令并把完整输出实时写入统一日志；参数为 argv/日志器/进程选项，返回无；非零退出码抛异常。"""
    logger.log("    [命令] " + " ".join(args))
    started = time.monotonic()
    process = subprocess.Popen(args, cwd=kwargs.pop("cwd", ROOT), stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               text=True, bufsize=1, **kwargs)
    for line in process.stdout:
        logger.log("    [命令输出] " + line.rstrip("\n"))
    status = process.wait()
    logger.log(f"    [命令] 退出={status}，耗时={time.monotonic() - started:.3f}s")
    if status:
        raise subprocess.CalledProcessError(status, args)


@contextmanager
def shared_run_lock(path=SHARED_LOCK):
    """用途：串行化共享数据库的构建与验收；参数为锁文件路径，返回上下文；竞争时立即失败，进程退出由系统释放锁。"""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    handle = open(path, "a+")
    try:
        try:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise RuntimeError("已有 make testdata 或 make e2e 正在操作共享 xhub 数据") from error
        handle.seek(0)
        handle.truncate()
        handle.write(str(os.getpid()) + "\n")
        handle.flush()
        yield
    finally:
        try:
            fcntl.flock(handle.fileno(), fcntl.LOCK_UN)
        finally:
            handle.close()


def sql(statement):
    """用途：操作隔离 PostgreSQL schema；参数为脚本，返回标准输出；仅 runner 生成标识，密码不进命令行。"""
    return command(["docker", "exec", "-i", "xhub-postgres", "psql", "-X", "-U", "xhub", "-d", "xhub", "-qAt", "-v", "ON_ERROR_STOP=1"],
                   input=statement, text=True, capture_output=True).stdout


def audit_response_usage(schema="public"):
    """用途：安全核对普通成功响应中的 usage 与计量事件；参数为受 runner 控制的 schema，返回分类计数；空正文、非法 JSON、非法 token 或数值不一致都会抛出断言，查询只读。"""
    row = sql(f"""SET search_path TO {schema};
WITH candidate_logs AS MATERIALIZED (
  SELECT u.request_id, u.prompt_tokens, u.completion_tokens,
    pg_input_is_valid(l.response_body, 'jsonb') AS valid_json,
    CASE WHEN pg_input_is_valid(l.response_body, 'jsonb') THEN l.response_body::jsonb END AS response
  FROM usage_events u JOIN request_logs l ON l.request_id=u.request_id
  WHERE u.status='success' AND u.call_type IN ('chat', 'responses')
    AND u.request_id NOT LIKE 'official-settlement:%'
), parsed_logs AS MATERIALIZED (
  SELECT *, response ? 'usage' AS has_usage,
    CASE WHEN pg_input_is_valid(coalesce(response->'usage'->>'prompt_tokens', response->'usage'->>'input_tokens'), 'bigint')
      THEN coalesce(response->'usage'->>'prompt_tokens', response->'usage'->>'input_tokens')::bigint END AS response_prompt_tokens,
    CASE WHEN pg_input_is_valid(coalesce(response->'usage'->>'completion_tokens', response->'usage'->>'output_tokens'), 'bigint')
      THEN coalesce(response->'usage'->>'completion_tokens', response->'usage'->>'output_tokens')::bigint END AS response_completion_tokens
  FROM candidate_logs
)
SELECT json_build_object(
  'successful_responses', count(*),
  'invalid_json', count(*) FILTER (WHERE NOT valid_json),
  'with_usage', count(*) FILTER (WHERE has_usage),
  'invalid_usage', count(*) FILTER (WHERE has_usage AND
    (response_prompt_tokens IS NULL OR response_completion_tokens IS NULL)),
  'mismatches', count(*) FILTER (WHERE has_usage AND
    response_prompt_tokens IS NOT NULL AND response_completion_tokens IS NOT NULL AND
    (response_prompt_tokens IS DISTINCT FROM prompt_tokens OR
     response_completion_tokens IS DISTINCT FROM completion_tokens))
) FROM parsed_logs;""").strip()
    result = json.loads(row)
    if result["invalid_json"] != 0:
        raise AssertionError("普通成功响应存在空正文或非法 JSON: " + str(result["invalid_json"]))
    if result["invalid_usage"] != 0:
        raise AssertionError("普通成功响应存在非法 usage token: " + str(result["invalid_usage"]))
    if result["with_usage"] == 0:
        raise AssertionError("普通成功响应中没有可核对的 usage")
    if result["mismatches"] != 0:
        raise AssertionError("历史真实响应usage与持久化计量不一致: " + str(result["mismatches"]))
    return result


def video_log_conditions(task):
    """用途：生成视频日志的精确任务、创建与结算条件；参数为报告任务，返回三个 SQL 条件；供恢复及计量审计共用，兼容原日志更新和历史独立结算。缺少模型、任务或不支持的协议抛 ValueError，无数据库副作用。"""
    if not task.get("model") or not task.get("task_id") or task.get("transport") not in (
            "qiniu_contents_generation", "qiniu_fal_doubao_20", "qiniu_fal_kling"):
        raise ValueError("视频日志恢复需要模型、Task ID 和支持的协议")
    model = str(task["model"]).replace("'", "''")
    task_id = str(task["task_id"]).replace("'", "''")
    transport = task["transport"]
    suffix = "get" if task["transport"] == "qiniu_contents_generation" else "status"
    # 正文精确匹配任务字段，避免 Task ID 前缀、prompt 或历史同模型任务污染证据。
    scope = f"""u.model='{model}' AND CASE WHEN pg_input_is_valid(l.response_body, 'jsonb')
      THEN coalesce(nullif(l.response_body::jsonb->>'id', ''), l.response_body::jsonb->>'request_id')='{task_id}'
      ELSE false END"""
    create = f"u.call_type='{transport}:create'"
    # 新协议在原创建事件上写 completed/task_settled；旧协议保留 success 的独立结算事件。
    settled = f"""(({create} AND u.status='completed' AND u.task_settled) OR
      (u.call_type='{transport}:{suffix}' AND u.status='success'
       AND u.request_id LIKE 'official-settlement:%'))"""
    return scope, create, settled


def audit_video_settlement(task, schema="public"):
    """用途：核对精确视频任务只有一次有价格及正费用的结算；参数为任务和受控 schema，返回计数及日志 ID；供数据库验收调用，兼容原日志完成与旧独立结算，缺失或重复均抛断言，只读查询。"""
    scope, _, settled = video_log_conditions(task)
    row = sql(f"""SET search_path TO {schema};
SELECT json_build_object('rows',count(*),'priced',count(*) FILTER (WHERE u.price_snapshot<>''),
 'positive',count(*) FILTER (WHERE u.cost>0),'request_id',coalesce(min(u.request_id),''))
FROM usage_events u JOIN request_logs l USING(request_id)
WHERE {scope} AND {settled};""").strip()
    proof = json.loads(row)
    if any(proof.get(field) != 1 for field in ("rows", "priced", "positive")) or not proof.get("request_id"):
        raise AssertionError("视频任务结算未去重或缺少价格快照: " + task["model"] + " " + json.dumps(proof))
    return proof


def recover_media_log_ids(dataset, schema="public"):
    """用途：按模型、协议与精确 Task ID 恢复视频日志；参数为数据集和受控 schema，返回任务数；原日志结算时两个 ID 相同，兼容旧独立结算。缺失证据抛断言，只读 PostgreSQL 并保存报告，无供应商调用。"""
    recovered = 0
    for task in dataset.report.get("media_tasks", []):
        scope, create, settled = video_log_conditions(task)
        row = sql(f"""SET search_path TO {schema};
SELECT coalesce(min(u.request_id) FILTER (WHERE {create}), '') || '|' ||
       coalesce(min(u.request_id) FILTER (WHERE {settled}), '')
FROM usage_events u JOIN request_logs l USING(request_id)
WHERE {scope};""").strip()
        create_id, separator, settlement_id = row.partition("|")
        if not separator or not create_id or not settlement_id:
            raise AssertionError("无法恢复视频创建或终态日志 ID: " + task["model"] + " task=" + task["task_id"])
        task["create_call_id"] = create_id
        task["settlement_log_id"] = settlement_id
        recovered += 1
        dataset.progress(f"    ✅ [{task['model']} / {task['transport']} / 恢复精确日志 ID] 创建={create_id}，终态={settlement_id}")
    dataset.save()
    return recovered


def free_port():
    """用途：选择可用回环端口；无参数，返回整数；供隔离服务启动，绑定竞争由健康检查和进程退出发现。"""
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def ready(url, process, timeout=90):
    """用途：等待服务可访问；参数为 URL/子进程/秒数，返回无；服务提前退出或超时明确失败，不复用其他任务服务。"""
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        if process.poll() is not None:
            raise RuntimeError("隔离服务启动失败，请查看运行目录日志")
        try:
            if module.http(url, "")[0] == 200:
                return
        except Exception:
            pass
        time.sleep(0.5)
    raise TimeoutError("隔离服务健康检查超时")


def validate_providers(data, environ=None, require_credentials=True):
    """用途：在清库或验收前验证供应商配置；参数为清单、可选环境及是否需要真实凭据，返回无；建数只校验地址，验收缺少密钥或任何阶段地址非法均立即失败，无外部请求。"""
    environ = os.environ if environ is None else environ
    for provider in data["providers"]:
        if require_credentials and not environ.get(provider["key_env"]):
            raise ValueError("缺少环境凭据: " + provider["key_env"])
        base = environ.get(provider["base_env"], provider["base"])
        url = module.urlsplit(base)
        if url.scheme != "https" or not url.hostname or url.username or url.password or any(c in base for c in "[]() \n\r\t"):
            raise ValueError("供应商地址必须是无 Markdown 的 HTTPS URL")


def validate_shared_target(database_url, redis_url):
    """用途：把破坏性清理限制到用户指定的 xhub 与 Redis DB 1；参数为两个 URL，返回无；任何差异都拒绝执行。"""
    if database_url != SHARED_DATABASE_URL:
        raise ValueError("共享验收只允许清理指定的 PostgreSQL xhub 数据库")
    if redis_url != SHARED_REDIS_URL:
        raise ValueError("共享验收只允许清理指定的 Redis DB 1")


def redis_command(*parts):
    """用途：向本机 Redis DB 1 发送受控命令；参数为命令片段，返回 RESP 值；仅供 SELECT/FLUSHDB/DBSIZE 清理核对。"""
    def encode(command_parts):
        """用途：编码单条 Redis RESP 命令；参数为命令片段，返回字节；只处理当前 runner 内部常量。"""
        request = b"*" + str(len(command_parts)).encode() + b"\r\n"
        for part in command_parts:
            value = str(part).encode()
            request += b"$" + str(len(value)).encode() + b"\r\n" + value + b"\r\n"
        return request

    def reply(stream):
        """用途：读取 Redis 简单字符串或整数响应；参数为二进制流，返回值；错误响应转为异常且不包含秘密。"""
        prefix, line = stream.read(1), stream.readline().rstrip(b"\r\n")
        if prefix == b"-":
            raise RuntimeError("Redis 命令失败: " + line.decode(errors="replace"))
        if prefix == b":":
            return int(line)
        if prefix == b"+":
            return line.decode()
        raise RuntimeError("Redis 返回了未支持的响应类型")

    with socket.create_connection(("127.0.0.1", 6379), timeout=10) as connection:
        stream = connection.makefile("rb")
        connection.sendall(encode(("SELECT", "1")) + encode(parts))
        if reply(stream) != "OK":
            raise RuntimeError("Redis DB 1 选择失败")
        return reply(stream)


def reset_shared_target(database_url=SHARED_DATABASE_URL, redis_url=SHARED_REDIS_URL, logger=None):
    """用途：重建指定 xhub 数据库并清空 Redis DB 1；参数为受白名单保护的 URL，返回无；会删除目标中的全部既有数据。"""
    validate_shared_target(database_url, redis_url)
    emit = logger or (lambda message: print(message, flush=True))
    emit("[testdata 1/5] 清空 PostgreSQL xhub 全部数据")
    psql = ["docker", "exec", "-i", "xhub-postgres", "psql", "-X", "-U", "xhub", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c"]
    command(psql + ["DROP DATABASE IF EXISTS xhub WITH (FORCE)"])
    command(psql + ["CREATE DATABASE xhub OWNER xhub"])
    emit("    PostgreSQL xhub 已重建，owner=xhub")
    emit("[testdata 2/5] 清空 Redis 127.0.0.1:6379/1 全部数据")
    if redis_command("FLUSHDB") != "OK":
        raise RuntimeError("Redis DB 1 清空失败")
    if redis_command("DBSIZE") != 0:
        raise RuntimeError("Redis DB 1 清空后仍有数据")
    emit("    Redis DB 1 已清空，DBSIZE=0")


def shared_config(password):
    """用途：生成直连用户指定数据库与 Redis 的网关配置；参数为管理员密码，返回配置字典；密钥仅写入私有运行目录。"""
    return {"model_list": [], "router_settings": {"num_retries": 1, "timeout": 90},
            "general_settings": {"master_key": "sk-" + secrets.token_hex(24), "admin_email": "admin", "admin_name": "admin",
            "admin_password": password, "store_model_in_db": True, "store_prompts_in_spend_logs": True,
            "database_url": SHARED_DATABASE_URL, "redis_url": SHARED_REDIS_URL}}




def run_browser(directory, env, gateway, ui_port, processes, handles, logger):
    """用途：启动真实前端并执行 Playwright 验收；参数为运行状态，返回浏览器报告；进程和日志句柄由调用方统一清理。"""
    frontend = ROOT / "frontend"
    tsconfig = json.loads((frontend / "tsconfig.json").read_text())
    tsconfig["include"] = [i for i in tsconfig["include"] if not i.startswith(".next")] + [".next-real-dataset/types/**/*.ts"]
    (frontend / "tsconfig.e2e.json").write_text(json.dumps(tsconfig, indent=2) + "\n")
    ui_env = {**env, "E2E_BUILD_DIR": ".next-real-dataset", "XHUB_GATEWAY_ORIGIN": gateway,
              "NEXT_PUBLIC_BASE_URL": gateway, "E2E_DATASET_GATEWAY": gateway,
              "E2E_DATASET_UI": "http://127.0.0.1:" + str(ui_port), "E2E_DATASET_DIR": str(directory)}
    log = open(directory / "console.log", "a")
    handles.append(log)
    ui = subprocess.Popen([str(frontend / "node_modules/.bin/next"), "dev", "-p", str(ui_port), "-H", "127.0.0.1"],
                          cwd=frontend, env=ui_env, stdout=log, stderr=log)
    processes.append(ui)
    logger.log(f"    [浏览器] 等待前端启动，地址=http://127.0.0.1:{ui_port}")
    ready(ui_env["E2E_DATASET_UI"] + "/ui/login/", ui, timeout=120)
    logger.log("    [浏览器] 前端就绪，启动 Chromium 真实页面验收")
    command_stream(["node", str(ROOT / "e2e/real_dataset_browser.cjs")], logger, env=ui_env)
    return json.loads((directory / "browser-report.json").read_text())


def run_business_browser(directory, logger):
    """用途：补齐去重浏览器业务；参数为报告目录和日志器，返回本轮统计；等待共享浏览器锁并在释放前保存专属证据，隔离库和模拟供应商由 e2e.sh 清理，失败或缺报告抛异常。"""
    titles = [
        "virtual key update, regenerate, block, and delete",
        "model update, test connection, and delete",
        "team member add is listed", "router fallback update lists the mapping",
        "admin panel saves prompt storage and hides the unused settings",
        "users can be edited and their team access follows membership changes",
        "关键词正则完成草稿调试、保存、编辑、真实拦截与删除",
        "XGo editor executes, persists, reloads, and edits real scripts",
        "chat explains missing credentials and unavailable deployment, then recovers",
        "comparison isolates card failures and supports recovery",
        "cancel and clear ignore delayed real responses; mobile chat remains usable",
        "错误日志详情完整显示上游正文并可返回普通日志",
        "异步任务原日志展示生命周期且轮询不增加日志：完成",
        "deployment deletion clears stale weights and preserves public inference",
        "deployment deletion clears default and template weights for inherited routing",
        "browser cache flush restores a billed miss after a free hit",
        "inherited session and key preview matches inference without consuming rpm",
        "every page route renders without a dashboard error",
        "the visibility chain holds at every tier",
    ]
    # 文件与精确标题同时约束，避免相近名称或参数化变体重复执行同一业务类型。
    files = ["writes.spec.ts", "user-edit.spec.ts", "guardrails.spec.ts", "xgo-guardrails.spec.ts",
             "playground-workspace.spec.ts", "error-logs.spec.ts", "task-request-logs.spec.ts",
             "route-diagnostic.spec.ts", "weighted-routing.spec.ts", "product-fixes.spec.ts",
             "coverage.spec.ts", "visibility-chain.spec.ts"]
    pattern = "(" + "|".join(re.escape(title) + "$" for title in titles) + ")"
    env = dict(os.environ)
    for key in ("E2E_LIVE", "E2E_CREDENTIAL_SOURCE"):
        env.pop(key, None)
    env["E2E_FULL_COVERAGE"] = "0"
    env["E2E_BROWSER_REPORT_DIR"] = str((directory / "business-browser").resolve())
    result_path = Path(env["E2E_BROWSER_REPORT_DIR"]) / "results.json"
    started = time.time()
    try:
        command_stream(["bash", "scripts/e2e.sh", *files, "--grep", pattern], logger, env=env)
    finally:
        # 即使失败也保存已执行及未执行的浏览器证据；e2e.sh 启动时删除旧报告。
        if result_path.exists() and result_path.stat().st_mtime >= started:
            shutil.copyfile(result_path, directory / "business-browser-results.json")
    if not result_path.exists() or result_path.stat().st_mtime < started:
        raise AssertionError("缺少本轮浏览器报告")
    stats = json.loads(result_path.read_text())["stats"]
    if stats["expected"] != len(titles) or any(stats[field] for field in ("unexpected", "flaky", "skipped")):
        raise AssertionError("去重浏览器流程未全部执行通过: " + str(stats))
    return dict(status="passed", cases=len(titles), stats=stats, report="business-browser-results.json")


def verify_database(dataset, schema="public"):
    """用途：核对聊天和媒体事件、日报、价格快照、去重与五级累计；参数为数据集和 schema，返回汇总；查询只读且失败立即中止验收。"""
    dataset.progress("    [数据库 1/6] 汇总成功事件、唯一请求和总金额，并与 usage_daily 对账")
    lines = sql(f"""SET search_path TO {schema};
SELECT json_build_object('events',count(*),'unique_calls',count(distinct request_id),'cost',coalesce(sum(cost),0)) FROM usage_events WHERE status IN ('success', 'completed');
SELECT coalesce(sum(cost),0) FROM usage_daily;
""").strip().splitlines()
    result = json.loads(lines[0])
    if abs(float(lines[1]) - result["cost"]) > 1e-9:
        raise AssertionError("每日聚合金额与真实请求事件不一致")
    dataset.progress(f"    ✅ [PostgreSQL / usage_daily / 总额对账] events={result['events']} unique_calls={result['unique_calls']} cost={result['cost']}")
    dataset.progress("    [数据库 2/6] 核对事件与每日汇总的请求数、输入 token、输出 token")
    counters = sql(f"SET search_path TO {schema}; SELECT json_build_object('requests',count(*),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_events; SELECT json_build_object('requests',coalesce(sum(requests),0),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_daily;").strip().splitlines()
    if json.loads(counters[0]) != json.loads(counters[1]):
        raise AssertionError("事件与每日汇总的请求数或token不一致")
    result["metering"] = json.loads(counters[0])
    dataset.progress(f"    ✅ [PostgreSQL / usage_events / 计量聚合] requests={result['metering']['requests']} prompt_tokens={result['metering']['prompt_tokens']} completion_tokens={result['metering']['completion_tokens']}")
    dataset.progress("    [数据库 3/6] 逐条核对含 usage 的普通响应与 usage_events")
    response_audit = audit_response_usage(schema)
    result["response_usage"] = response_audit
    dataset.progress(f"    ✅ [PostgreSQL / request_logs / 响应用量核对] 成功响应={response_audit['successful_responses']} 含usage={response_audit['with_usage']} 非法JSON=0 非法usage=0 用量不一致=0")
    dataset.progress("    [数据库 4/6] 精确核对 gpt-image-2 请求事件、价格快照和正费用")
    for call in dataset.report.get("media_calls", []):
        call_id = str(call["call_id"]).replace("'", "''")
        row = sql(f"""SET search_path TO {schema};
SELECT json_build_object('rows',count(*),'priced',count(*) FILTER (WHERE price_snapshot<>''),'positive',count(*) FILTER (WHERE cost>0))
FROM usage_events WHERE status='success' AND request_id='{call_id}';""").strip()
        proof = json.loads(row)
        if proof != {"rows": 1, "priced": 1, "positive": 1}:
            raise AssertionError("图片请求缺少唯一结算、价格快照或正费用: " + json.dumps(proof))
        result["image_settlement"] = proof
        dataset.progress(f"    ✅ [{call['model']} / bypass_openai_image_generation / 图片结算] 1 个真实请求、1 条 usage event、价格快照非空、费用为正")
    dataset.progress("    [数据库 5/6] 核对三种视频每个真实任务只有一条成功结算及价格快照")
    result["media_settlements"] = {}
    for task in dataset.report.get("media_tasks", []):
        proof = audit_video_settlement(task, schema)
        task["settlement_log_id"] = proof["request_id"]
        result["media_settlements"][task["model"]] = proof
        dataset.progress(f"    ✅ [{task['model']} / {task['transport']} / 结算去重] 1 个任务、1 条结算日志、3 次重复终态查询未重复扣费")
    dataset.progress("    [数据库 6/6] 核对组织、团队、用户、项目、密钥五级累计金额")
    for table, column, dimension in (("organizations", "id", "organization_id"), ("teams", "id", "team_id"),
                                     ("users", "id", "user_id"), ("projects", "id", "project_id"), ("api_keys", "id", "key_id")):
        mismatch = sql(f"SET search_path TO {schema}; SELECT count(*) FROM {table} x WHERE abs(x.spend - coalesce((SELECT sum(cost) FROM usage_events u WHERE u.{dimension}=x.{column}),0)) > 0.000000001;").strip()
        if mismatch != "0":
            raise AssertionError("累计计费不一致: " + table)
        dataset.progress(f"      ✅ [PostgreSQL / {table} / 累计金额] 不一致记录=0")
    dataset.report["checks"].append({"name": "postgres-events-daily-five-owner-spend", "passed": True})
    return result


def recover_shared_chat_checkpoints(dataset):
    """用途：从共享 PostgreSQL 恢复中断前已成功的 81 密钥聊天证据；参数为已加载状态的数据集，返回恢复数量；只接受真实 dataset 标记、choices、响应 usage、价格快照、模型和五级归属均匹配的每密钥最新记录。"""
    statement = """SELECT json_build_object(
 'request_id',u.request_id,'api_key',u.key_id,'user',u.user_id,'team_id',u.team_id,
 'project_id',u.project_id,'organization_id',u.organization_id,'model',u.model,
 'prompt_tokens',u.prompt_tokens,'completion_tokens',u.completion_tokens,'spend',u.cost,
 'metadata',json_build_object('cost_breakdown',u.price_snapshot),
 'messages',l.request_body::jsonb,'response',l.response_body::jsonb)
FROM usage_events u JOIN request_logs l USING(request_id)
WHERE u.status='success' AND u.call_type='chat' AND l.request_body LIKE '%dataset-%'
  AND u.prompt_tokens > 0 AND u.completion_tokens > 0 AND u.price_snapshot <> ''
ORDER BY u.ts DESC;"""
    rows = [json.loads(line) for line in sql(statement).splitlines() if line.strip()]
    keys = {key["token_id"]: key for key in dataset.state["keys"]}
    recovered = {}
    for bill in rows:
        key = keys.get(bill.get("api_key"))
        if not key or key["token_id"] in recovered or bill.get("model") != key.get("call_model"):
            continue
        messages, response = bill.get("messages", []), bill.get("response", {})
        match = re.search(r"dataset-[0-9a-f]+-\d+", json.dumps(messages, ensure_ascii=False))
        usage = response.get("usage", {}) if isinstance(response, dict) else {}
        try:
            cost = module.bill_check(bill, key, dataset.data["billing"])
            if not match or not response.get("choices") or any(
                    int(bill.get(field, 0)) != int(usage.get(field, -1))
                    for field in ("prompt_tokens", "completion_tokens")):
                continue
        except (AssertionError, TypeError, ValueError):
            continue
        recovered[key["token_id"]] = {"call_id": bill["request_id"], "marker": match.group(0),
            "key_id": key["token_id"], "profile": key["profile"], "model": key["call_model"],
            "prompt_tokens": bill["prompt_tokens"], "completion_tokens": bill["completion_tokens"], "cost": cost}
    existing = {row.get("key_id"): row for row in dataset.report.get("calls", [])
                if row.get("key_id") in keys and row.get("key_id") not in recovered}
    dataset.report["calls"] = [recovered.get(key["token_id"]) or existing.get(key["token_id"])
                               for key in dataset.state["keys"]
                               if recovered.get(key["token_id"]) or existing.get(key["token_id"])]
    dataset.save()
    dataset.progress(f"    ✅ [PostgreSQL / request_logs / 中断恢复] 已恢复并预校验 {len(recovered)}/81 把密钥的真实调用证据；运行期将逐条通过日志接口复验")
    return len(recovered)


def restore_supplier_addresses(dataset, data):
    """用途：停止观察器前恢复仍存在部署的真实 HTTPS 地址；参数为数据集和清单，返回无；逐项按供应商 ID 恢复并打印结果，清除旧错误，仅把本次真实失败留在报告。"""
    if not dataset or not dataset.admin:
        return
    dataset.report.pop("retention_error", None)
    failures = []
    for deployment in dataset.state.get("deployments", []):
        if deployment.get("kind") != "chat" or not deployment.get("observed"):
            continue
        provider = next((p for p in data["providers"] if p["id"] == deployment.get("provider")), None)
        if not provider:
            failures.append(deployment.get("id", "unknown") + " 引用了未知供应商")
            continue
        try:
            dataset.api("/model/update", {"model_info": {"id": deployment["id"]},
                "litellm_params": {"api_base": os.environ.get(provider["base_env"], provider["base"])}})
            dataset.action_ok(deployment.get("public_name", deployment["id"]), "bypass_openai_chat",
                              "恢复供应商地址", f"部署 ID={deployment['id']}，供应商={provider['id']}", record=False)
        except Exception as error:
            # 生命周期和故障注入部署可能已在 finally 中删除；只有这类明确的临时部署可忽略 404。
            if deployment.get("temporary") and "状态 404" in str(error):
                dataset.action_ok(deployment.get("public_name", deployment["id"]), "bypass_openai_chat",
                                  "恢复供应商地址", f"临时部署 {deployment['id']} 已删除，无需恢复", record=False)
                continue
            failures.append(deployment.get("id", "unknown") + " 恢复失败: " + type(error).__name__)
            dataset.action_fail(deployment.get("public_name", deployment.get("id", "unknown")),
                                "bypass_openai_chat", "恢复供应商地址", error)
    if failures:
        dataset.report["retention_error"] = "；".join(failures)


def attach_chat_observer(dataset, observer):
    """用途：把聊天部署接入观察器并补齐旧基线的 Responses 声明；参数为数据集和观察器，返回无；供共享和隔离续跑使用，媒体仍直连真实供应商。"""
    for deployment in dataset.state.get("deployments", []):
        if not deployment.get("observed"):
            continue
        deployment["endpoint_types"] = ["chat", "responses"]
        dataset.api("/model/update", {"model_info": {"id": deployment["id"], "endpoint_types": deployment["endpoint_types"]},
            "litellm_params": {"api_base": observer.base + "/" + deployment["provider"]}})


def shared_main(args, data):
    """用途：分阶段构建并验收 public 真实数据；参数为 CLI 和清单，返回退出码；seed 清空目标，verify 保留基线并执行浏览器/计量/回归。"""
    try:
        with shared_run_lock():
            return shared_main_locked(args, data)
    except RuntimeError as error:
        print(str(error), file=sys.stderr)
        return 1


def shared_main_locked(args, data):
    """用途：持锁执行共享数据构建或验收；参数为 CLI 和清单，返回退出码；仅由 shared_main 调用并覆盖完整服务生命周期。"""
    try:
        validate_providers(data, require_credentials=args.phase != "seed")
        validate_shared_target(SHARED_DATABASE_URL, SHARED_REDIS_URL)
    except ValueError as error:
        raise SystemExit(str(error))
    directory = Path(args.directory).resolve()
    if args.phase == "seed":
        if directory.exists():
            shutil.rmtree(directory)
        directory.mkdir(parents=True)
        os.chmod(directory, 0o700)
        logger = RunLogger(directory / "testdata.log")
        logger.log("目标：PostgreSQL xhub/public；Redis DB 1；阶段=testdata")
        reset_shared_target(logger=logger.log)
        password = secrets.token_urlsafe(24)
        config = shared_config(password)
        module.write_private(directory / "c.yaml", config)
        (directory / "schema.txt").write_text("public\n")
    else:
        required = [directory / name for name in ("c.yaml", "access.json", "report.json", "schema.txt")]
        if any(not path.exists() for path in required):
            raise SystemExit("缺少 make testdata 基线，请先运行 make testdata")
        config = json.loads((directory / "c.yaml").read_text())
        validate_shared_target(config["general_settings"].get("database_url"), config["general_settings"].get("redis_url"))
        if (directory / "schema.txt").read_text().strip() != "public":
            raise SystemExit("共享验收状态不是 public schema")
        password = config["general_settings"]["admin_password"]
        logger = RunLogger(directory / "e2e.log")
        logger.log("目标：PostgreSQL xhub/public；Redis DB 1；阶段=e2e")
    gateway_port, ui_port = free_port(), free_port()
    gateway = "http://127.0.0.1:" + str(gateway_port)
    env = {**os.environ, "E2E_DATASET_PASSWORD": password, "E2E_DATASET_ADMIN": "admin", "XHUB_PUBLIC_ORIGIN": gateway}
    processes, handles, observer, dataset = [], [], None, None
    try:
        logger.log("[testdata 3/5] 构建并启动真实 xhub 网关" if args.phase == "seed" else "[e2e 1/4] 构建并启动真实 xhub 网关")
        build_started = time.monotonic()
        command(["go", "build", "-o", str(directory / "xhub"), "./cmd/gateway"])
        logger.log(f"    网关编译完成，耗时={time.monotonic() - build_started:.3f}s")
        log = open(directory / "gateway.log", "a")
        handles.append(log)
        gw = subprocess.Popen([str(directory / "xhub"), "-config", str(directory / "c.yaml"), "-addr", f"127.0.0.1:{gateway_port}"], cwd=ROOT, env=env, stdout=log, stderr=log)
        processes.append(gw)
        ready(gateway + "/health/liveliness", gw)
        logger.log(f"    网关健康检查通过，地址={gateway}")
        if args.phase == "verify":
            observer = module.Observer(data["providers"], data["limits"]["max_upstream_attempts"],
                                       logger=lambda message: logger.log("    [真实上游] " + message))
        os.environ["E2E_DATASET_PASSWORD"] = password
        os.environ["E2E_DATASET_ADMIN"] = "admin"
        dataset = module.Dataset(gateway, directory, data, observer, logger=logger.log)
        if args.phase == "seed":
            logger.log("[testdata 4/5] 仅通过管理接口构建 real-acceptance 数据，不调用模型")
            dataset.seed()
            dataset.report["counts"] = {name: len(dataset.state[name]) for name in ("organizations", "teams", "users", "projects", "keys")}
            dataset.report["status"] = "seeded"
            dataset.report["target"] = {"database_url": SHARED_DATABASE_URL, "redis_url": SHARED_REDIS_URL, "schema": "public"}
            dataset.save()
            logger.log("[testdata 5/5] 构建完成：3组织 / 9团队 / 27成员 / 9项目 / 81成员密钥 + 1管理员个人密钥（共82把）/ 模型、路由和护栏配置；模型调用=0。Codex 三轮会话、图片及视频由 make e2e 执行")
        else:
            saved = json.loads((directory / "access.json").read_text())
            dataset.state = {key: value for key, value in saved.items() if key not in ("admin", "gateway")}
            dataset.report = json.loads((directory / "report.json").read_text())
            # 兼容旧报告：候选探测是模型选择诊断，不是最终验收项；迁移后正式 checks 只表示必须通过的断言。
            legacy_probes = [row for row in dataset.report.get("checks", []) if row.get("name") == "real-model-probe"]
            dataset.report["checks"] = [row for row in dataset.report.get("checks", []) if row.get("name") != "real-model-probe"]
            dataset.report.setdefault("probe_attempts", []).extend(legacy_probes)
            dataset.report.update(status="running", phase="verify")
            dataset.report.pop("error", None)
            dataset.representative = True
            dataset.start_checklist(args.with_regression)
            login, _ = dataset.api("/v2/login", {"username": "admin", "password": password})
            dataset.admin = login["key"]
            recover_shared_chat_checkpoints(dataset)
            recover_media_log_ids(dataset)
            attach_chat_observer(dataset, observer)
            logger.log("[e2e 2/4] 保留81密钥基线，按4种路由各选1把验收；逐项显示内容和结果")
            dataset.verify()
            # API 复验完成后先用数据库精确恢复媒体终态日志，再让浏览器按 ID 打开同一证据。
            recover_media_log_ids(dataset)
            dataset.run_case("media-logs", dataset.verify_saved_media_logs)
            logger.log("[e2e 3/4] 执行 PostgreSQL 计量核对与真实浏览器验收")
            dataset.report["database"] = dataset.run_case("database", verify_database, dataset)
            dataset.save()
            dataset.report["browser"] = dataset.run_case("browser", run_browser, directory, env, gateway, ui_port, processes, handles, logger)
            dataset.report["business_browser"] = dataset.run_case("business-browser", run_business_browser, directory, logger)
            dataset.save()
            if args.with_regression:
                logger.log("[e2e 4/4] 执行后台 regression 测试")
                dataset.run_case("regression", command_stream, ["bash", "scripts/regression.sh", "-v"], logger)
                dataset.report["backend_regression"] = {"command": "bash scripts/regression.sh -v", "status": "passed"}
            dataset.report["status"] = "passed"
            dataset.save()
            logger.log("真实验收通过，报告: " + str(directory / "report.json"))
    except Exception as error:
        if dataset:
            dataset.report.update(status="failed", error=type(error).__name__ + ": " + str(error))
            dataset.save()
        logger.log("❌ [real-acceptance / runner / 验收失败] " + type(error).__name__ + ": " + str(error) + "；日志目录: " + str(directory))
        return 1
    finally:
        if observer:
            restore_supplier_addresses(dataset, data)
        if dataset:
            dataset.save()
            if dataset.checklist:
                dataset.checklist.summary()
        for process in reversed(processes):
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if observer:
            evidence = directory / "upstream-observations.json"
            earlier = json.loads(evidence.read_text()) if evidence.exists() else []
            evidence.write_text(json.dumps(earlier + observer.rows, ensure_ascii=False, indent=2) + "\n")
            observer.close()
        for handle in handles:
            handle.close()
        (directory / "retained.txt").write_text("保留 PostgreSQL xhub/public 与 Redis DB 1，供 make e2e 和人工复核。\n")
    return 0


def main():
    """用途：一条命令完成真实数据验收；参数来自 CLI/环境，返回退出码；默认清理 schema/Redis，keep-data 仅保留明确创建的基线。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--keep-data", action="store_true")
    parser.add_argument("--api-only", action="store_true", help="只运行后台真实请求验收，报告明确浏览器未执行")
    parser.add_argument("--resume", action="store_true", help="复用同一目录已保存基线和81请求证据，续跑新增场景与浏览器")
    parser.add_argument("--directory", default=str(ROOT / ".e2e" / ("real-dataset-" + time.strftime("%Y%m%d-%H%M%S"))))
    parser.add_argument("--shared-target", action="store_true", help="直连并保留用户指定的 xhub/public 与 Redis DB 1")
    parser.add_argument("--phase", choices=("seed", "verify"), help="共享目标分为构建数据和验收数据两个阶段")
    parser.add_argument("--with-regression", action="store_true", help="验收阶段追加后台 regression 套件")
    args = parser.parse_args()
    if args.shared_target:
        if not args.phase:
            parser.error("--shared-target 必须指定 --phase seed 或 verify")
        return shared_main(args, module.load_manifest())
    if args.phase or args.with_regression:
        parser.error("--phase/--with-regression 只能与 --shared-target 一起使用")
    if args.resume and not args.keep_data:
        parser.error("续跑必须同时指定 --keep-data，避免删除已有验收基线")
    data = module.load_manifest()
    for provider in data["providers"]:
        if not os.environ.get(provider["key_env"]):
            raise SystemExit("缺少环境凭据: " + provider["key_env"])
        base = os.environ.get(provider["base_env"], provider["base"])
        url = module.urlsplit(base)
        if url.scheme != "https" or not url.hostname or url.username or url.password or any(c in base for c in "[]() \n\r\t"):
            raise SystemExit("供应商地址必须是无 Markdown 的 HTTPS URL")
    directory = Path(args.directory).resolve()
    if not args.resume and (directory / "c.yaml").exists():
        raise SystemExit("目录已包含运行基线；请使用 --resume --keep-data 或新目录")
    directory.mkdir(parents=True, exist_ok=True)
    os.chmod(directory, 0o700)
    suffix = secrets.token_hex(6)
    schema, redis_name = "e2e_real_" + suffix, "xhub-real-" + suffix
    gateway_port, ui_port, redis_port = free_port(), free_port(), free_port()
    password = secrets.token_urlsafe(24)
    env = {**os.environ, "E2E_DATASET_PASSWORD": password, "E2E_DATASET_ADMIN": "admin"}
    gateway = "http://127.0.0.1:" + str(gateway_port)
    env["XHUB_PUBLIC_ORIGIN"] = gateway
    config = {"model_list": [], "router_settings": {"num_retries": 1, "timeout": 90},
              "general_settings": {"master_key": "sk-" + secrets.token_hex(24), "admin_email": "admin", "admin_name": "admin",
              "admin_password": password, "store_model_in_db": True, "store_prompts_in_spend_logs": True,
              "database_url": "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable&search_path=" + schema,
              "redis_url": "redis://127.0.0.1:" + str(redis_port) + "/0"}}
    if args.resume:
        config = json.loads((directory / "c.yaml").read_text())
        schema = (directory / "schema.txt").read_text().strip()
        if not module.re.fullmatch(r"e2e_real_[a-f0-9]{12}", schema):
            raise ValueError("续跑只允许本入口创建的隔离schema")
        config["general_settings"]["redis_url"] = "redis://127.0.0.1:" + str(redis_port) + "/0"
        password = config["general_settings"]["admin_password"]
        env["E2E_DATASET_PASSWORD"] = password
    processes, observer, dataset = [], None, None
    schema_created = redis_created = False
    handles = []
    try:
        command(["go", "build", "-o", str(directory / "xhub"), "./cmd/gateway"])
        if not args.resume:
            sql("CREATE SCHEMA " + schema)
        schema_created = True
        command(["docker", "run", "--rm", "-d", "--name", redis_name, "-p", f"127.0.0.1:{redis_port}:6379", "redis:7-alpine"], capture_output=True)
        redis_created = True
        module.write_private(directory / "c.yaml", config)
        (directory / "schema.txt").write_text(schema + "\n")
        log = open(directory / "gateway.log", "a")
        handles.append(log)
        gw = subprocess.Popen([str(directory / "xhub"), "-config", str(directory / "c.yaml"), "-addr", f"127.0.0.1:{gateway_port}"],
                              cwd=ROOT, env=env, stdout=log, stderr=log)
        processes.append(gw)
        ready(gateway + "/health/liveliness", gw)
        observer = module.Observer(data["providers"], data["limits"]["max_upstream_attempts"])
        if args.resume and (directory / "upstream-observations.json").exists():
            observer.attempts = len(json.loads((directory / "upstream-observations.json").read_text()))
        os.environ["E2E_DATASET_PASSWORD"] = password
        os.environ["E2E_DATASET_ADMIN"] = "admin"
        dataset = module.Dataset(gateway, directory, data, observer)
        if args.resume:
            saved = json.loads((directory / "access.json").read_text())
            dataset.state = {k: v for k, v in saved.items() if k not in ("admin", "gateway")}
            dataset.report = json.loads((directory / "report.json").read_text())
            dataset.report["status"] = "running"
            dataset.report.pop("error", None)
            if not any(c["name"] == "81-personal-keys-real-calls-and-billing" and c["passed"] for c in dataset.report["checks"]):
                raise ValueError("续跑要求已有81把真实请求及账单证据")
            login, _ = dataset.api("/v2/login", {"username": "admin", "password": password})
            dataset.admin = login["key"]
            # 中断的回退验收可能已完成请求而尚未清理，续跑先删除明确标记的临时故障部署。
            for deployment in list(dataset.state["deployments"]):
                if deployment.get("temporary_fault"):
                    dataset.api("/model/fallback", {"model_name": deployment["public_name"], "policy": {}}, method="PUT")
                    dataset.api("/model/delete", {"id": deployment["id"]})
                    dataset.state["deployments"].remove(deployment)
            allowed_models = [deployment["public_name"] for deployment in dataset.state["deployments"]]
            for team in dataset.state["teams"]:
                dataset.api("/team/update", {"team_id": team["id"], "models": allowed_models})
            attach_chat_observer(dataset, observer)
            dataset.verify_acceptance_models()
            if not all(dataset.checked("permissions-" + role["role"]) for role in data["personas"]):
                dataset.verify_permissions()
            key = dataset.state["keys"][0]
            dataset.verify_limits(key)
            dataset.verify_model_lifecycle(key)
            dataset.verify_guardrail_lifecycle(key)
            dataset.save()
        else:
            dataset.seed()
            print("真实数据创建完成：3组织 / 9团队 / 27成员 / 81密钥；开始请求验收", flush=True)
            dataset.verify()
        if args.api_only:
            dataset.report["browser"] = "not-run (--api-only)"
        else:
            frontend = ROOT / "frontend"
            # 独立运行不依赖其他E2E脚本预先生成的忽略文件。
            tsconfig = json.loads((frontend / "tsconfig.json").read_text())
            tsconfig["include"] = [i for i in tsconfig["include"] if not i.startswith(".next")] + [".next-real-dataset/types/**/*.ts"]
            (frontend / "tsconfig.e2e.json").write_text(json.dumps(tsconfig, indent=2) + "\n")
            ui_env = {**env, "E2E_BUILD_DIR": ".next-real-dataset", "XHUB_GATEWAY_ORIGIN": gateway,
                      "NEXT_PUBLIC_BASE_URL": gateway, "E2E_DATASET_GATEWAY": gateway,
                      "E2E_DATASET_UI": "http://127.0.0.1:" + str(ui_port), "E2E_DATASET_DIR": str(directory)}
            log = open(directory / "console.log", "a")
            handles.append(log)
            ui = subprocess.Popen([str(frontend / "node_modules/.bin/next"), "dev", "-p", str(ui_port), "-H", "127.0.0.1"],
                                  cwd=frontend, env=ui_env, stdout=log, stderr=log)
            processes.append(ui)
            ready(ui_env["E2E_DATASET_UI"] + "/ui/login/", ui, timeout=120)
            command(["node", str(ROOT / "e2e/real_dataset_browser.cjs")], env=ui_env)
            dataset.report["browser"] = json.loads((directory / "browser-report.json").read_text())
        # SQL 验收覆盖每日汇总与实体累计金额，失败时仍保留报告；不读取或输出原始请求/凭据。
        statement = f"""SET search_path TO {schema};
SELECT json_build_object('events',count(*),'unique_calls',count(distinct request_id),'cost',coalesce(sum(cost),0)) FROM usage_events WHERE status='success';
SELECT coalesce(sum(cost),0) FROM usage_daily;
"""
        lines = sql(statement).strip().splitlines()
        dataset.report["database"] = json.loads(lines[0])
        if abs(float(lines[1]) - dataset.report["database"]["cost"]) > 1e-9:
            raise AssertionError("每日聚合金额与真实请求事件不一致")
        counters = sql(f"SET search_path TO {schema}; SELECT json_build_object('requests',count(*),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_events; SELECT json_build_object('requests',coalesce(sum(requests),0),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_daily;").strip().splitlines()
        if json.loads(counters[0]) != json.loads(counters[1]):
            raise AssertionError("事件与每日汇总的请求数或token不一致")
        dataset.report["database"]["metering"] = json.loads(counters[0])
        # 续跑也重新读取历史真实响应：81个已有证据不能只依赖上一轮的通过标记。
        dataset.report["database"]["response_usage"] = audit_response_usage(schema)
        for table, column, dimension in (("organizations", "id", "organization_id"), ("teams", "id", "team_id"),
                                         ("users", "id", "user_id"), ("projects", "id", "project_id"), ("api_keys", "id", "key_id")):
            mismatch = sql(f"SET search_path TO {schema}; SELECT count(*) FROM {table} x WHERE abs(x.spend - coalesce((SELECT sum(cost) FROM usage_events u WHERE u.{dimension}=x.{column}),0)) > 0.000000001;").strip()
            if mismatch != "0":
                raise AssertionError("累计计费不一致: " + table)
        dataset.report["checks"].append({"name": "postgres-events-daily-five-owner-spend", "passed": True})
        dataset.report["status"] = "passed"
        dataset.save()
        print("真实验收通过，报告: " + str(directory / "report.json"), flush=True)
    except Exception as error:
        if dataset:
            dataset.report.update(status="failed", error=str(error))
            dataset.save()
        print("验收失败: " + str(error) + "；日志目录: " + str(directory), file=sys.stderr)
        return 1
    finally:
        if dataset and args.keep_data:
            # 观察器退出后基线仍可调用；只恢复曾经改写到观察器的聊天部署。
            restore_supplier_addresses(dataset, data)
            dataset.save()
        for process in reversed(processes):
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if observer:
            evidence = directory / "upstream-observations.json"
            earlier = json.loads(evidence.read_text()) if args.resume and evidence.exists() else []
            evidence.write_text(json.dumps(earlier + observer.rows, ensure_ascii=False, indent=2))
            observer.close()
        for handle in handles:
            handle.close()
        if redis_created:
            subprocess.run(["docker", "stop", redis_name], capture_output=True)
        if schema_created and not args.keep_data:
            sql("DROP SCHEMA " + schema + " CASCADE")
            (directory / "cleanup.txt").write_text("隔离 PostgreSQL schema 和 Redis 容器已删除；保留报告。\n")
        if args.keep_data:
            (directory / "retained.txt").write_text("保留 schema: " + schema + "\n重新启动前为 c.yaml 配置可用 Redis 并导出供应商环境凭据。\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
