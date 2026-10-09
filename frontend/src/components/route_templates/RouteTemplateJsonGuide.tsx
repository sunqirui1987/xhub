"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

/** 可导入示例包含模型规则、路由组、回退与完整可靠性参数；不引用部署 ID 或供应商密钥。
 * JSON 指南和契约测试共用；模型名仅为示例，不创建模型目录或发起请求。 */
export const ROUTE_TEMPLATE_EXAMPLE = {
  model_routes: [{ model: "chat-main", strategy: "least-busy" }],
  retry_policy: { max_attempts: 2, timeout_seconds: 60, failure_threshold: 3, cooldown_seconds: 60 },
  routing_groups: [
    { group_name: "chat-group", models: ["chat-main", "chat-fast"], routing_strategy: "cost-based-routing" },
  ],
  fallbacks: [{ "chat-group": ["chat-backup"] }],
  context_window_fallbacks: [{ "chat-main": ["chat-long"] }],
  content_policy_fallbacks: [],
};
const fields = [
  ["model_routes", "array · 必填", "空数组继承模型管理", "按公开模型配置负载均衡，每个模型最多一条。优先于组策略。"],
  ["model_routes[].model", "string · 必填", "非空且唯一", "客户端调用的公开模型名，不是上游型号或访问白名单。"],
  [
    "model_routes[].strategy",
    "string · 必填",
    "下方策略枚举",
    "控制该模型内部的部署选择，traffic-split 可配置模板独立权重或继承模型默认。",
  ],
  [
    "model_routes[].allocations",
    "array · 可选",
    "仅 traffic-split",
    "省略继承模型默认；[] 使用均等权重1；有值时仅当前模板生效。",
  ],
  [
    "model_routes[].allocations[].deployment_id",
    "string · 必填",
    "该模型的真实唯一部署ID",
    "表单展示提供商、上游型号和协议；JSON使用稳定ID关联。",
  ],
  [
    "model_routes[].allocations[].weight",
    "number · 必填",
    "有限非负，非空列表至少一个正值",
    "相对权重；0排除部署；未列出的部署默认1，不读取默认权重。",
  ],
  ["retry_policy", "object · 必填", "四个字段完整", "对所有目标部署应用重试、超时和被动冷却。"],
  [
    "retry_policy.max_attempts",
    "number · 正整数",
    "≥1，默认1",
    "每条部署总尝试次数，包含首次。对应LiteLLM额外重试次数加1，不是整条链总上限。",
  ],
  ["retry_policy.timeout_seconds", "number · 正数", ">0，默认60秒", "单次上游超时；允许小数，不是整条链的时间预算。"],
  ["retry_policy.failure_threshold", "number · 非负整数", "默认3；0禁用", "部署失败被动冷却阈值，共享状态依赖Redis。"],
  [
    "retry_policy.cooldown_seconds",
    "number · 非负数",
    "默认60秒；0也使用60秒",
    "冷却持续时间；用failure_threshold=0禁用冷却。",
  ],
  ["routing_groups", "array · 可选", "新模板为空数组", "组随模板保存、复制与绑定；旧模板缺失字段时兼容历史全局组。"],
  [
    "routing_groups[].group_name",
    "string · 必填",
    "1–64字符，不能空白、星号或default",
    "可作为API的model调用，不可与公开模型或本模板其他组重名。",
  ],
  [
    "routing_groups[].models",
    "string[] · 必填",
    "至少1项、确切公开模型",
    "组名调用展开所有成员；成员名调用只选该成员。每模型在当前模板最多属于一组。",
  ],
  [
    "routing_groups[].routing_strategy",
    "string · 必填",
    "下方策略枚举",
    "组自己的负载均衡策略，未命中模型规则时生效。",
  ],
  [
    "routing_groups[].routing_strategy_args",
    "object · 可选",
    "仅traffic-split可用",
    "包含allocations；空列表使用部署权重1。",
  ],
  [
    "routing_groups[].routing_strategy_args.allocations[].deployment_id",
    "string · 必填",
    "真实且唯一的组内部署ID",
    "稳定部署ID，不能填写模型名。",
  ],
  [
    "routing_groups[].routing_strategy_args.allocations[].weight",
    "number · 必填",
    "有限非负，非空配置至少一个正权重",
    "相对权重，无需合计100；0排除部署，未列出的部署默认1。",
  ],
  ["fallbacks", "array · 可选", "[{主模型:[目标1,目标2]}]", "429、5xx和传输失败；未配置的主模型继承模型管理默认链。"],
  ["context_window_fallbacks", "array · 可选", "同上，源在类别内唯一", "识别上游上下文超限错误，使用独立有序链。"],
  [
    "content_policy_fallbacks",
    "array · 可选",
    "同上，每条最多32目标",
    "识别上游内容策略错误；不把本地护栏拦截当作回退。",
  ],
  [
    "回退映射的主模型与目标",
    "string / string[]",
    "公开模型或本模板组名",
    "顺序保留、禁止重复/自引用/跨类别循环。[]继承默认；[{主模型:[]}]禁用该主模型对应链。",
  ],
];
const strategies = [
  ["simple-shuffle", "简单随机", "兼容部署均匀随机选择，不读取部署权重。新建推荐。random 是同义值。"],
  [
    "traffic-split",
    "按流量分流",
    "优先使用模板独立相对权重；模型规则省略 allocations 才继承默认。无需合计 100；未列出部署权重为 1，显式 0 排除。",
  ],
  ["least-busy", "最少并发", "优先当前处理请求最少的部署；多实例共享观测依赖 Redis。"],
  ["latency-based-routing", "最低延迟", "按观测延迟选择部署；不是预先填写固定延迟，统计依赖 Redis。"],
  ["cost-based-routing", "最低成本", "按当前输入 token 单价比较部署，不估算整次请求总价。依赖模型目录中的费率。"],
  ["usage-based-routing", "最低用量", "按已统计 token 用量选择部署；不是 RPM/TPM 剩余额度排序，统计依赖 Redis。"],
];
const examples = [
  {
    title: "独立配置模型端点权重",
    body: {
      ...ROUTE_TEMPLATE_EXAMPLE,
      model_routes: [
        {
          model: "chat-main",
          strategy: "traffic-split",
          allocations: [
            { deployment_id: "真实部署ID-1", weight: 3 },
            { deployment_id: "真实部署ID-2", weight: 7 },
          ],
        },
      ],
    },
  },
  { title: "完整模板示例", body: ROUTE_TEMPLATE_EXAMPLE },
  {
    title: "按权重分流的路由组",
    body: {
      ...ROUTE_TEMPLATE_EXAMPLE,
      routing_groups: [
        {
          group_name: "weighted-chat",
          models: ["chat-main"],
          routing_strategy: "traffic-split",
          routing_strategy_args: {
            allocations: [
              { deployment_id: "真实部署ID-1", weight: 3 },
              { deployment_id: "真实部署ID-2", weight: 7 },
            ],
          },
        },
      ],
      fallbacks: [],
    },
  },
  {
    title: "保留模型管理的权重和回退",
    body: {
      model_routes: [],
      retry_policy: ROUTE_TEMPLATE_EXAMPLE.retry_policy,
      routing_groups: [],
      fallbacks: [],
      context_window_fallbacks: [],
      content_policy_fallbacks: [],
    },
  },
];

/** JsonExample 渲染可复制的完整文档；参数为标题及合法对象，返回代码和操作反馈。
 * 指南示例分区调用；剪贴板不可用时显示错误，不更新编辑草稿、不访问后台。 */
function JsonExample({ title, body }: { title: string; body: Record<string, unknown> }) {
  const [status, setStatus] = useState("");
  const json = JSON.stringify(body, null, 2);
  /** copy 将当前示例写入系统剪贴板；无参数，失败反馈可见，返回完成的异步操作。 */
  async function copy() {
    try {
      await navigator.clipboard.writeText(json);
      setStatus("已复制");
    } catch {
      setStatus("复制失败，请手动选择代码复制");
    }
  }
  return (
    <section aria-label={title} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="font-medium">{title}</h4>
        <Button variant="outline" size="sm" onClick={() => void copy()}>
          复制 JSON
        </Button>
      </div>
      <pre className="overflow-x-auto rounded-lg border bg-muted/30 p-4 text-xs leading-5">{json}</pre>
      <p role="status" className="text-xs text-muted-foreground">
        {status}
      </p>
    </section>
  );
}

/** RouteTemplateJsonGuide 提供完整 JSON 帮助；无参数，返回分区字段表、示例和兼容性说明。
 * 模板页的大弹窗调用；仅复制示例，不执行配置。仅展示当前严格模板契约。 */
export default function RouteTemplateJsonGuide() {
  return (
    <div className="space-y-4 pb-4 text-sm">
      <h3 className="text-base font-semibold">路由模板 JSON 指南</h3>
      <Tabs defaultValue="fields">
        <TabsList className="h-auto flex-wrap justify-start" aria-label="JSON 指南分区">
          <TabsTrigger value="fields">完整字段</TabsTrigger>
          <TabsTrigger value="strategies">策略说明</TabsTrigger>
          <TabsTrigger value="examples">可导入示例</TabsTrigger>
          <TabsTrigger value="execution">执行与绑定</TabsTrigger>
          <TabsTrigger value="litellm">LiteLLM 对照</TabsTrigger>
          <TabsTrigger value="validation">校验与错误</TabsTrigger>
        </TabsList>
        <TabsContent value="fields" className="space-y-4 pt-3">
          <p>
            一份模板正文包含负载均衡规则、可靠性参数、路由组和三类回退。模板名称和组织、团队、个人密钥绑定在管理接口保存。所有字段都有对应表单。
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <caption className="sr-only">当前支持的模板字段</caption>
              <thead className="border-b">
                <tr>
                  <th className="p-2">字段</th>
                  <th className="p-2">类型与必填</th>
                  <th className="p-2">边界 / 默认</th>
                  <th className="p-2">执行含义</th>
                </tr>
              </thead>
              <tbody>
                {fields.map(([name, type, bounds, meaning]) => (
                  <tr key={name} className="border-b align-top">
                    <td className="break-words p-2 font-mono">{name}</td>
                    <td className="p-2">{type}</td>
                    <td className="p-2">{bounds}</td>
                    <td className="p-2">{meaning}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-muted-foreground">
            默认值说明新建表单的初始设置；导入时不会替你补全必填字段。所有数值必须是 JSON number，不能写成字符串。
          </p>
        </TabsContent>
        <TabsContent value="strategies" className="space-y-4 pt-3">
          {strategies.map(([id, label, meaning]) => (
            <section key={id} className="border-b pb-3">
              <h4 className="font-medium">
                {label} <code className="ml-2 text-xs">{id}</code>
              </h4>
              <p className="mt-2 text-muted-foreground">{meaning}</p>
            </section>
          ))}
          <p>
            会话固定部署优先于重新选择策略，但部署仍须兼容且可用。没有可用候选时返回失败，不会为了保活绕过零权重或冷却限制。
          </p>
        </TabsContent>
        <TabsContent value="examples" className="space-y-6 pt-3">
          <p>这些是可以导入的完整 xhub 模板正文。示例模型名需替换为你的公开模型名；复制不会自动应用到草稿。</p>
          {examples.map((example) => (
            <JsonExample key={example.title} {...example} />
          ))}
        </TabsContent>
        <TabsContent value="execution" className="space-y-4 pt-3">
          <h4 className="font-medium">一次请求怎样选择路由</h4>
          <ol className="list-decimal space-y-3 pl-5">
            <li>确定绑定模板：密钥 → 团队 → 组织 → 内置默认。选择第一份可用模板，整份替换，不逐字段合并。</li>
            <li>
              解析模型策略：精确命中的 model_routes → routing_groups → 模型管理默认权重（旧模板保留 routing_strategy
              兼容）。未列出的模型仍可调用。
            </li>
            <li>
              按公开模型、入口协议和部署能力筛选候选，再按策略应用权重及可用状态。部署凭据、地址和费率在模型管理中维护；权重可在模板独立设置。
            </li>
            <li>优先有效的会话固定部署，再执行部署池调度。按每部署 max_attempts 与单次 timeout_seconds 执行请求。</li>
            <li>
              429、5xx
              和传输失败可以重试或切换同模型部署。流式内容发出后不会重新开始响应；部署池耗尽后执行模板的对应错误回退链；未覆盖的源继承模型管理默认回退。组别名和实际成员均检查权限与预算，真实成员负责计费。
            </li>
          </ol>
          <p>
            保存模板后，仅绑定该模板的范围使用新配置；未绑定的草稿不会影响请求。预览使用后台真实规则解析，但不会调用上游或产生
            token 费用。
          </p>
        </TabsContent>
        <TabsContent value="litellm" className="space-y-4 pt-3">
          <p>
            xhub 借鉴 LiteLLM 的功能分区。这里的 JSON 是 xhub 模板契约，不能直接导入 LiteLLM 的 router_settings 或
            YAML。
          </p>
          <table className="w-full text-left">
            <caption className="sr-only">LiteLLM 能力支持对照</caption>
            <thead>
              <tr className="border-b">
                <th className="p-2">能力</th>
                <th className="p-2">当前实现与差异</th>
              </tr>
            </thead>
            <tbody>
              <tr className="border-b">
                <td className="p-2">负载均衡</td>
                <td className="p-2">
                  已支持上列策略。simple-shuffle 在 xhub 中为均匀随机；traffic-split 单独读取部署权重。用量策略不是
                  LiteLLM 的 RPM/TPM 调度。
                </td>
              </tr>
              <tr className="border-b">
                <td className="p-2">重试与冷却</td>
                <td className="p-2">
                  已支持。max_attempts 包含首次调用；与 LiteLLM num_retries 的额外重试计数不同。failure_threshold
                  也不能直接等同于 LiteLLM 的 allowed_fails。
                </td>
              </tr>
              <tr className="border-b">
                <td className="p-2">模型回退</td>
                <td className="p-2">
                  模板内配置通用、上下文与内容策略三类有序链，映射格式参考LiteLLM。未覆盖的源与类别继承模型管理默认回退。
                </td>
              </tr>
            </tbody>
          </table>
          <p>后台会拒绝未知字段；每个可导入字段均有对应表单。</p>
          <div className="flex flex-wrap gap-3">
            {[
              ["routing", "路由概览"],
              ["proxy/load_balancing", "负载均衡"],
              ["proxy/reliability", "可靠性"],
            ].map(([path, label]) => (
              <a
                key={path}
                className="underline underline-offset-4"
                href={"https://docs.litellm.com.cn/docs/" + path}
                target="_blank"
                rel="noreferrer"
              >
                {label}
              </a>
            ))}
          </div>
        </TabsContent>
        <TabsContent value="validation" className="space-y-4 pt-3">
          <h4 className="font-medium">常见错误与修正</h4>
          <ul className="list-disc space-y-3 pl-5">
            <li>
              未知字段：删除 num_retries、model_list、router_settings、endpoint_id、顶层 allocations 等 LiteLLM
              或旧版字段；不要直接复制其他系统配置。
            </li>
            <li>不完整对象：model_routes 和 retry_policy 必须存在，可靠性四个字段不能缺少。空规则写 []。</li>
            <li>错误类型：使用 60，不能使用 "60"；不能使用 null、NaN、Infinity、负数或小数尝试次数。</li>
            <li>重复模型或策略拼写错误：同一公开模型仅保留一条；策略使用“策略说明”中的精确标识。</li>
            <li>
              JSON 语法错误：不要写注释、尾随逗号或单引号。表单和 JSON 双向同步；非法 JSON
              会禁用保存和切回表单，修正后恢复。
            </li>
            <li>
              没有候选：检查模型是否存在、入口协议是否兼容、部署是否冷却，以及权重是否全部为 0。合法 JSON
              不保证该模型已有可用部署。
            </li>
          </ul>
        </TabsContent>
      </Tabs>
    </div>
  );
}
