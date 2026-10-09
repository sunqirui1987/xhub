import { Search, ArrowUpRight, Code, ShieldCheck, Filter } from "lucide-react";
import { useState } from "react";
import { t } from "@/i18n";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { PARTNER_GUARDRAIL_CARDS } from "./guardrail_garden_data";
import { CARD_PROVIDERS, EXTERNAL_PROVIDERS, externalGuardrailText } from "./externalProviders";
import ExternalGuardrailEditor from "./ExternalGuardrailEditor";
interface Props {
  accessToken: string | null;
  onGuardrailCreated: () => void;
  onCreateLocal: () => void;
  onCreateXGo: () => void;
}
/**
 * 用途：保留伙伴服务目录并按实际适配能力显示已接入、远端接入或尚未移植。
 * 参数：Props：访问令牌、创建成功刷新、本地规则和 XGo 编辑器打开回调。
 * 返回：可搜索的目录、服务配置表单或未移植服务的远端接入说明。
 * 调用：GuardrailsPanel 的护栏花园标签页。
 * 测试：guardrail_garden.integration.test.tsx。
 */
export default function GuardrailGarden({ accessToken, onGuardrailCreated, onCreateLocal, onCreateXGo }: Props) {
  const [query, setQuery] = useState("");
  const [support, setSupport] = useState("all");
  const [selected, setSelected] = useState<string | null>(null);
  const [bridgeService, setBridgeService] = useState<string | null>(null);
  // 保留原有伙伴目录；仅 CARD_PROVIDERS 有映射的项目可进入本地适配器配置。
  // 其余项目提供远端 LiteLLM 桥接，不能仅凭卡片存在宣称已支持。
  const cards = [
    ...PARTNER_GUARDRAIL_CARDS,
    {
      id: "azure_content_safety",
      name: "Azure Content Safety",
      description: EXTERNAL_PROVIDERS["azure/text_moderations"].description,
      tags: ["Microsoft", "Safety"],
    },
    {
      id: "azure_prompt_shield",
      name: "Azure Prompt Shield",
      description: EXTERNAL_PROVIDERS["azure/prompt_shield"].description,
      tags: ["Microsoft", "Prompt injection"],
    },
    {
      id: "secret_detection",
      name: "Secret detection",
      description: EXTERNAL_PROVIDERS["hide-secrets"].description,
      tags: ["Secrets", "Local"],
    },
    {
      id: "litellm_proxy",
      name: "Remote LiteLLM",
      description: EXTERNAL_PROVIDERS.litellm_proxy.description,
      tags: ["Remote", "LiteLLM"],
    },
  ];
  // 支持状态由实际协议映射决定；搜索和筛选共用同一列表，空结果提供明确反馈。
  const visibleCards = cards.filter((card) => {
    const provider = CARD_PROVIDERS[card.id];
    let status = "pending";
    if (provider) status = "native";
    if (provider === "litellm_proxy") status = "remote";
    return (
      (support === "all" || support === status) &&
      [card.name, card.description, ...card.tags].join(" ").toLowerCase().includes(query.trim().toLowerCase())
    );
  });
  if (selected && EXTERNAL_PROVIDERS[selected])
    return (
      <div className="p-4">
        <Button
          variant="ghost"
          onClick={() => {
            setSelected(null);
            setBridgeService(null);
          }}
        >
          {t("返回护栏花园")}
        </Button>
        {bridgeService && (
          <p className="my-3">
            {externalGuardrailText("请先在远端 LiteLLM 配置 {service}，再填写该护栏的远端名称。", {
              service: bridgeService,
            })}
          </p>
        )}
        <ExternalGuardrailEditor
          provider={selected}
          accessToken={accessToken}
          onSuccess={onGuardrailCreated}
          onClose={() => {
            setSelected(null);
            setBridgeService(null);
          }}
        />
      </div>
    );
  const selectedCard = cards.find((card) => card.id === selected);
  if (selectedCard)
    return (
      <div className="space-y-4 p-4">
        <Button variant="ghost" onClick={() => setSelected(null)}>
          {t("返回护栏花园")}
        </Button>
        <h2 className="text-xl font-semibold">{selectedCard.name}</h2>
        <p>{externalGuardrailText(selectedCard.description)}</p>
        <p>{externalGuardrailText("尚未移植：可以通过远端 LiteLLM 使用该服务。")}</p>
        <p>{t("XHub 不会将该目录项当成已实现的本地适配器。")}</p>
        <Button
          disabled={!accessToken}
          onClick={() => {
            setBridgeService(selectedCard.name);
            setSelected("litellm_proxy");
          }}
        >
          {externalGuardrailText("配置远端 LiteLLM")}
        </Button>
        <a
          className="block text-primary underline"
          href="https://docs.litellm.com.cn/docs/proxy/guardrails/quick_start"
          target="_blank"
          rel="noreferrer"
        >
          {externalGuardrailText("查看 LiteLLM 护栏文档")}
        </a>
      </div>
    );
  return (
    <div className="space-y-7 py-4">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">{t("选择护栏实现")}</h2>
        <p className="mt-2 text-sm text-muted-foreground">{t("先选择实现方式，完成配置与测试，再启用护栏。")}</p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <button
          className="group rounded-xl border bg-card p-6 text-left transition hover:border-primary/40 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          onClick={onCreateLocal}
          disabled={!accessToken}
        >
          <div className="mb-4 flex items-center justify-between">
            <span className="flex size-10 items-center justify-center rounded-xl bg-primary/5 text-primary">
              <ShieldCheck className="size-5" />
            </span>
            <ArrowUpRight className="size-4 text-muted-foreground group-hover:text-primary" />
          </div>
          <h3 className="text-base font-semibold">{t("关键词 / 正则护栏")}</h3>
          <p className="mt-2 text-sm text-muted-foreground">{t("本地执行，支持敏感词拦截和正则脱敏。")}</p>
        </button>
        <button
          className="group rounded-xl border bg-card p-6 text-left transition hover:border-primary/40 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          onClick={onCreateXGo}
          disabled={!accessToken}
        >
          <div className="mb-4 flex items-center justify-between">
            <span className="flex size-10 items-center justify-center rounded-xl bg-primary/5 text-primary">
              <Code className="size-5" />
            </span>
            <ArrowUpRight className="size-4 text-muted-foreground group-hover:text-primary" />
          </div>
          <h3 className="text-base font-semibold">{t("XGo 自定义护栏")}</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("通过 xhub/guardrail 使用 HTTP、JSON、Flag 与大模型函数。")}
          </p>
        </button>
      </div>
      <div className="border-t pt-7">
        <h2 className="text-lg font-semibold tracking-tight">{t("外部服务目录")}</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {t("连接内容安全与隐私服务；支持状态按当前适配能力展示。")}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full sm:max-w-sm">
          <Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
          <Input
            aria-label={t("搜索护栏服务")}
            placeholder={t("搜索护栏服务")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="bg-card pl-9"
          />
        </div>
        <div className="flex items-center gap-2">
          <Filter aria-hidden="true" className="size-4 text-muted-foreground" />
          <select
            aria-label={t("服务支持状态")}
            value={support}
            onChange={(e) => setSupport(e.target.value)}
            className="h-9 rounded-md border bg-card px-3 text-sm"
          >
            <option value="all">{t("全部服务")}</option>
            <option value="native">{t("已接入")}</option>
            <option value="remote">{t("远端接入")}</option>
            <option value="pending">{t("尚未移植")}</option>
          </select>
        </div>
      </div>
      {visibleCards.length === 0 && (
        <p role="status" className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {t("没有匹配的护栏服务")}
        </p>
      )}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {visibleCards.map((card) => {
          const provider = CARD_PROVIDERS[card.id];
          const supported = !!provider;
          let supportLabel = "尚未移植";
          if (supported) supportLabel = "已接入";
          if (provider === "litellm_proxy") supportLabel = "远端接入";
          return (
            <button
              key={card.id}
              className="group flex min-h-48 min-w-0 flex-col rounded-xl border bg-card p-5 text-left transition hover:border-primary/40 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => setSelected(provider ?? card.id)}
            >
              <span className="flex w-full items-center justify-between gap-2">
                <span className="min-w-0 text-sm font-semibold">{card.name}</span>
                <span
                  className={
                    supported
                      ? "shrink-0 whitespace-nowrap rounded-full bg-emerald-500/10 px-2 py-1 text-[11px] font-medium text-emerald-700 dark:text-emerald-400"
                      : "shrink-0 whitespace-nowrap rounded-full bg-muted px-2 py-1 text-[11px] text-muted-foreground"
                  }
                >
                  {t(supportLabel)}
                </span>
              </span>
              <span className="mt-3 line-clamp-3 text-xs leading-5 text-muted-foreground">
                {externalGuardrailText(EXTERNAL_PROVIDERS[provider]?.description ?? card.description)}
              </span>
              <span className="mt-auto flex items-center justify-between pt-4 text-xs font-medium text-primary">
                {t(supported ? "配置与测试" : "查看接入方式")}
                <ArrowUpRight className="size-3.5" />
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
