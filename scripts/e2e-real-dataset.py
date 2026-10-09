#!/usr/bin/env python3
"""自动启动私有网关、真实供应商观察器和浏览器，运行 docs 数据集并清理。"""
import argparse
from contextlib import contextmanager
import fcntl
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
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


def validate_providers(data, environ=None):
    """用途：在任何清库动作前验证真实供应商配置；参数为清单和可选环境，返回无；缺少密钥或非法地址立即失败。"""
    environ = os.environ if environ is None else environ
    for provider in data["providers"]:
        if not environ.get(provider["key_env"]):
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


def verify_database(dataset, schema="public"):
    """用途：核对事件、日报、真实响应 token 与五级累计金额；参数为数据集和 schema，返回汇总；查询只读且失败立即中止验收。"""
    dataset.progress("    [数据库 1/4] 汇总成功事件、唯一请求和总金额，并与 usage_daily 对账")
    lines = sql(f"""SET search_path TO {schema};
SELECT json_build_object('events',count(*),'unique_calls',count(distinct request_id),'cost',coalesce(sum(cost),0)) FROM usage_events WHERE status='success';
SELECT coalesce(sum(cost),0) FROM usage_daily;
""").strip().splitlines()
    result = json.loads(lines[0])
    if abs(float(lines[1]) - result["cost"]) > 1e-9:
        raise AssertionError("每日聚合金额与真实请求事件不一致")
    dataset.progress(f"    [数据库 1/4] 通过：events={result['events']} unique_calls={result['unique_calls']} cost={result['cost']}")
    dataset.progress("    [数据库 2/4] 核对事件与每日汇总的请求数、输入 token、输出 token")
    counters = sql(f"SET search_path TO {schema}; SELECT json_build_object('requests',count(*),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_events; SELECT json_build_object('requests',coalesce(sum(requests),0),'prompt_tokens',coalesce(sum(prompt_tokens),0),'completion_tokens',coalesce(sum(completion_tokens),0)) FROM usage_daily;").strip().splitlines()
    if json.loads(counters[0]) != json.loads(counters[1]):
        raise AssertionError("事件与每日汇总的请求数或token不一致")
    result["metering"] = json.loads(counters[0])
    dataset.progress(f"    [数据库 2/4] 通过：requests={result['metering']['requests']} prompt_tokens={result['metering']['prompt_tokens']} completion_tokens={result['metering']['completion_tokens']}")
    dataset.progress("    [数据库 3/4] 逐条核对 request_logs 原始响应 usage 与 usage_events")
    mismatch = sql(f"""SET search_path TO {schema}; SELECT count(*) FROM usage_events u
JOIN request_logs l ON l.request_id=u.request_id WHERE u.status='success' AND
((l.response_body::jsonb->'usage'->>'prompt_tokens')::bigint IS DISTINCT FROM u.prompt_tokens
OR (l.response_body::jsonb->'usage'->>'completion_tokens')::bigint IS DISTINCT FROM u.completion_tokens);""").strip()
    if mismatch != "0":
        raise AssertionError("历史真实响应usage与持久化计量不一致")
    dataset.progress("    [数据库 3/4] 通过：usage 不一致记录=0")
    dataset.progress("    [数据库 4/4] 核对组织、团队、用户、项目、密钥五级累计金额")
    for table, column, dimension in (("organizations", "id", "organization_id"), ("teams", "id", "team_id"),
                                     ("users", "id", "user_id"), ("projects", "id", "project_id"), ("api_keys", "id", "key_id")):
        mismatch = sql(f"SET search_path TO {schema}; SELECT count(*) FROM {table} x WHERE abs(x.spend - coalesce((SELECT sum(cost) FROM usage_events u WHERE u.{dimension}=x.{column}),0)) > 0.000000001;").strip()
        if mismatch != "0":
            raise AssertionError("累计计费不一致: " + table)
        dataset.progress(f"      [五级金额] {table} 不一致记录=0")
    dataset.report["checks"].append({"name": "postgres-events-daily-five-owner-spend", "passed": True})
    return result


def restore_supplier_addresses(dataset, data):
    """用途：停止观察器前恢复基线部署的真实 HTTPS 地址；参数为数据集和清单，返回无；失败记录到报告供人工诊断。"""
    if not dataset or not dataset.admin:
        return
    aliases = (data["routing"]["alias"], data["routing"]["backup_alias"])
    for deployment in dataset.state.get("deployments", []):
        if deployment["alias"] not in aliases:
            continue
        provider = next(p for p in data["providers"] if p["id"] == deployment["provider"])
        try:
            dataset.api("/model/update", {"model_info": {"id": deployment["id"]},
                "litellm_params": {"api_base": os.environ.get(provider["base_env"], provider["base"])}})
        except Exception:
            dataset.report["retention_error"] = "恢复供应商地址失败，需查看私有部署配置"


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
        validate_providers(data)
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
        observer = module.Observer(data["providers"], data["limits"]["max_upstream_attempts"],
                                   logger=lambda message: logger.log("    [真实上游] " + message))
        os.environ["E2E_DATASET_PASSWORD"] = password
        os.environ["E2E_DATASET_ADMIN"] = "admin"
        dataset = module.Dataset(gateway, directory, data, observer, logger=logger.log)
        if args.phase == "seed":
            logger.log("[testdata 4/5] 探测真实供应商并构建 real-acceptance 数据")
            dataset.seed()
            dataset.report["counts"] = {name: len(dataset.state[name]) for name in ("organizations", "teams", "users", "projects", "keys")}
            dataset.report["status"] = "seeded"
            dataset.report["target"] = {"database_url": SHARED_DATABASE_URL, "redis_url": SHARED_REDIS_URL, "schema": "public"}
            dataset.save()
            logger.log("[testdata 5/5] 构建完成：3组织 / 9团队 / 27成员 / 9项目 / 81密钥")
        else:
            saved = json.loads((directory / "access.json").read_text())
            dataset.state = {key: value for key, value in saved.items() if key not in ("admin", "gateway")}
            dataset.report = json.loads((directory / "report.json").read_text())
            dataset.report.update(status="running", phase="verify")
            dataset.report.pop("error", None)
            login, _ = dataset.api("/v2/login", {"username": "admin", "password": password})
            dataset.admin = login["key"]
            for deployment in dataset.state["deployments"]:
                dataset.api("/model/update", {"model_info": {"id": deployment["id"]}, "litellm_params": {"api_base": observer.base + "/" + deployment["provider"]}})
            logger.log("[e2e 2/4] 执行真实 API、81 密钥、权限、预算、限流、护栏与回退验收")
            dataset.verify()
            logger.log("[e2e 3/4] 执行真实浏览器验收与 PostgreSQL 计量核对")
            dataset.report["browser"] = run_browser(directory, env, gateway, ui_port, processes, handles, logger)
            dataset.report["database"] = verify_database(dataset)
            dataset.save()
            if args.with_regression:
                logger.log("[e2e 4/4] 执行后台 regression 测试")
                command_stream(["bash", "scripts/regression.sh", "-v"], logger)
                dataset.report["backend_regression"] = {"command": "bash scripts/regression.sh -v", "status": "passed"}
            dataset.report["status"] = "passed"
            dataset.save()
            logger.log("真实验收通过，报告: " + str(directory / "report.json"))
    except Exception as error:
        if dataset:
            dataset.report.update(status="failed", error=type(error).__name__ + ": " + str(error))
            dataset.save()
        logger.log("验收失败: " + type(error).__name__ + ": " + str(error) + "；日志目录: " + str(directory))
        return 1
    finally:
        restore_supplier_addresses(dataset, data)
        if dataset:
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
            # 中断的回退验收可能已完成请求而尚未清理策略，续跑先按依赖顺序清理临时部署。
            for deployment in list(dataset.state["deployments"]):
                if deployment["alias"] == "验收限流回退":
                    dataset.api("/model/fallback", {"model_name": deployment["alias"], "policy": {}}, method="PUT")
                    dataset.api("/model/delete", {"id": deployment["id"]})
                    dataset.state["deployments"].remove(deployment)
            for team in dataset.state["teams"]:
                dataset.api("/team/update", {"team_id": team["id"], "models": [data["routing"]["alias"], data["routing"]["backup_alias"]]})
            for deployment in dataset.state["deployments"]:
                dataset.api("/model/update", {"model_info": {"id": deployment["id"]},
                    "litellm_params": {"api_base": observer.base + "/" + deployment["provider"]}})
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
        mismatch = sql(f"""SET search_path TO {schema}; SELECT count(*) FROM usage_events u
JOIN request_logs l ON l.request_id=u.request_id WHERE u.status='success' AND
((l.response_body::jsonb->'usage'->>'prompt_tokens')::bigint IS DISTINCT FROM u.prompt_tokens
OR (l.response_body::jsonb->'usage'->>'completion_tokens')::bigint IS DISTINCT FROM u.completion_tokens);""").strip()
        if mismatch != "0":
            raise AssertionError("历史真实响应usage与持久化计量不一致")
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
            # 观察器退出后基线仍可调用；将持久化部署恢复为原始供应商地址，凭据继续使用环境引用。
            for deployment in dataset.state["deployments"]:
                if deployment["alias"] not in (data["routing"]["alias"], data["routing"]["backup_alias"]):
                    continue
                provider = next(p for p in data["providers"] if p["id"] == deployment["provider"])
                try:
                    dataset.api("/model/update", {"model_info": {"id": deployment["id"]},
                        "litellm_params": {"api_base": os.environ.get(provider["base_env"], provider["base"])}})
                except Exception:
                    dataset.report["retention_error"] = "恢复供应商地址失败，需查看私有部署配置"
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
