"use client";

import { diagnoseRequest, RequestDiagnostic, type RequestFailure } from "../RequestDiagnostic";
import { guardCallbacks } from "../../hooks/requestScope";
import { toast } from "@/lib/toast";
import { DEBOUNCE_WAIT_MS } from "@/utils/debounceConstants";
import { Eraser, FileText, Plus, Trash2 } from "lucide-react";
import { useDebouncedValue } from "@tanstack/react-pacer/debouncer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useEffect, useMemo, useState, useRef } from "react";
import { v4 as uuidv4 } from "uuid";
import ChatImageUpload from "../chat_ui/ChatImageUpload";
import { createChatDisplayMessage, createChatMultimodalMessage } from "../chat_ui/ChatImageUtils";
import type { TokenUsage } from "@/components/chat_ui/ResponseMetrics";
import type { MessageType, VectorStoreSearchResponse } from "@/components/chat_ui/types";
import { makeOpenAIChatCompletionRequest } from "@/components/llm_calls/chat_completion";
import { getProxyBaseUrl } from "@/components/networking";
import { endpointLabel, callTextEndpoint, selectEndpoint, textEndpoints } from "@/components/llm_calls/model_endpoints";
import type { ModelGroup } from "@/components/llm_calls/fetch_models";
import { fetchAvailableModels } from "@/components/llm_calls/fetch_models";
import { Agent, fetchAvailableAgents } from "../../llm_calls/fetch_agents";
import { makeA2AStreamMessageRequest } from "../../llm_calls/a2a_send_message";
import { ComparisonPanel } from "./components/ComparisonPanel";
import { MessageInput } from "./components/MessageInput";
import {
  EndpointId,
  EndpointIdType,
  getAvailableEndpoints,
  getEndpointConfig,
  isAgentEndpoint,
  hasValidSelection,
  modelOptionsToSelectorOptions,
  agentOptionsToSelectorOptions,
} from "./endpoint_config";
import { t } from "@/i18n";
/** ComparisonInstance 保存一张对比卡片的模型、真实端点路径与独立请求状态。
 * endpoint 只从所选模型的明确文本绑定中选择；卡片之间不共享协议选择。
 */
export interface ComparisonInstance {
  id: string;
  model: string;
  endpoint?: string | null;
  agent: string;
  messages: MessageType[];
  isLoading: boolean;
  tags: string[];
  mcpTools: string[];
  vectorStores: string[];
  guardrails: string[];
  temperature: number;
  maxTokens: number;
  applyAcrossModels: boolean;
  useAdvancedParams: boolean;
  traceId?: string;
  failure?: RequestFailure | null;
}
interface CompareUIProps {
  accessToken: string | null;
  disabledPersonalKeyCreation: boolean;
}
const GENERIC_FOLLOW_UPS = [
  "Can you summarize the key points?",
  "What assumptions did you make?",
  "What are the next steps?",
];
const SUGGESTED_PROMPTS = ["Write me a poem", "Explain quantum computing", "Draft a polite email requesting a meeting"];
const DEFAULT_ENDPOINT = EndpointId.CHAT_COMPLETIONS;
/** CompareUI 为每个模型独立选择支持的文本端点，并并行显示输出与实测用量。
 * 参数 accessToken：会话令牌；disabledPersonalKeyCreation：是否要求自定义测试密钥。
 * 返回：模型对比界面；模型先选择，端点后校验，不使用 mode 推断调用方式。
 * 调用：Playground 对比标签。测试：CompareUI.test.tsx。
 */
export default function CompareUI({ accessToken, disabledPersonalKeyCreation }: CompareUIProps) {
  const requests = useRef(new Map<string, AbortController>());
  const preparing = useRef(false);
  const requestHistories = useRef(new Map<string, MessageType[]>());
  const generation = useRef(0);
  // 同一草稿部分失败时记录已完成卡片，重试只补发失败卡；文本、附件或上下文改变后开始新一轮。
  const retryRound = useRef<{ text: string; file: File | null; completed: Set<string> } | null>(null);
  /** stopRequests 取消指定或全部卡片请求并解除身份；参数可选卡片 ID，返回无；清空和切换调用。 */
  const stopRequests = (id?: string) => {
    generation.current++;
    retryRound.current = null;
    for (const [key, controller] of requests.current) {
      if (!id || key === id) {
        controller.abort();
        requests.current.delete(key);
      }
    }
    // 停止恢复请求前历史，避免把未完成的用户轮次或半截回答带入重试。
    const histories = new Map(requestHistories.current);
    for (const key of histories.keys()) if (!id || key === id) requestHistories.current.delete(key);
    setComparisons((previous) =>
      previous.map((card) =>
        !id || card.id === id ? { ...card, messages: histories.get(card.id) ?? card.messages, isLoading: false } : card,
      ),
    );
  };
  useEffect(
    () => () => {
      generation.current++;
      for (const controller of requests.current.values()) controller.abort();
      requests.current.clear();
      requestHistories.current.clear();
    },
    [],
  );
  const [comparisons, setComparisons] = useState<ComparisonInstance[]>([
    {
      id: "1",
      model: "",
      agent: "",
      messages: [],
      isLoading: false,
      tags: [],
      mcpTools: [],
      vectorStores: [],
      guardrails: [],
      temperature: 1,
      maxTokens: 2048,
      applyAcrossModels: false,
      useAdvancedParams: false,
    },
    {
      id: "2",
      model: "",
      agent: "",
      messages: [],
      isLoading: false,
      tags: [],
      mcpTools: [],
      vectorStores: [],
      guardrails: [],
      temperature: 1,
      maxTokens: 2048,
      applyAcrossModels: false,
      useAdvancedParams: false,
    },
  ]);
  const [modelFailure, setModelFailure] = useState<RequestFailure | null>(null);
  const [modelInfo, setModelInfo] = useState<ModelGroup[]>([]);
  const [modelOptions, setModelOptions] = useState<string[]>([]);
  const [agentOptions, setAgentOptions] = useState<Agent[]>([]);
  const [isLoadingModels, setIsLoadingModels] = useState(false);
  const [isLoadingAgents, setIsLoadingAgents] = useState(false);
  const [selectedEndpoint, setSelectedEndpoint] = useState<EndpointIdType>(DEFAULT_ENDPOINT);

  // Derived state from endpoint config
  const endpointConfig = getEndpointConfig(selectedEndpoint);
  const isA2AMode = isAgentEndpoint(selectedEndpoint);
  const selectorOptions = isA2AMode
    ? agentOptionsToSelectorOptions(agentOptions)
    : modelOptionsToSelectorOptions(modelOptions);
  const isLoadingOptions = isA2AMode ? isLoadingAgents : isLoadingModels;
  const [inputValue, setInputValue] = useState("");
  const [uploadedFile, setUploadedFile] = useState<File | null>(null);
  const [uploadedFilePreviewUrl, setUploadedFilePreviewUrl] = useState<string | null>(null);
  const [apiKeySource, setApiKeySource] = useState<"session" | "custom">(
    disabledPersonalKeyCreation ? "custom" : "session",
  );
  const [customApiKey, setCustomApiKey] = useState("");
  const [debouncedCustomApiKey] = useDebouncedValue(customApiKey, { wait: DEBOUNCE_WAIT_MS });
  const [customProxyBaseUrl] = useState<string>(() => sessionStorage.getItem("customProxyBaseUrl") || "");
  useEffect(() => {
    return () => {
      if (uploadedFilePreviewUrl) {
        URL.revokeObjectURL(uploadedFilePreviewUrl);
      }
    };
  }, [uploadedFilePreviewUrl]);
  const effectiveApiKey = useMemo(
    () => (apiKeySource === "session" ? accessToken || "" : debouncedCustomApiKey.trim()),
    [apiKeySource, accessToken, debouncedCustomApiKey],
  );
  const compareContext = [effectiveApiKey, selectedEndpoint, customProxyBaseUrl].join("\n");
  const previousContext = useRef(compareContext);
  useEffect(() => {
    if (previousContext.current !== compareContext) {
      previousContext.current = compareContext;
      generation.current++;
      for (const controller of requests.current.values()) controller.abort();
      requests.current.clear();
      setComparisons((cards) =>
        cards.map((card) => ({ ...card, messages: [], traceId: undefined, failure: null, isLoading: false })),
      );
    }
  }, [compareContext]);
  const haveAllResponses = useMemo(
    () =>
      comparisons.length > 0 &&
      comparisons.every(
        (comparison) => !comparison.isLoading && comparison.messages.some((message) => message.role === "assistant"),
      ),
    [comparisons],
  );
  useEffect(() => {
    let active = true;
    const loadModels = async () => {
      if (!effectiveApiKey) {
        setModelOptions([]);
        setModelInfo([]);
        return;
      }
      setIsLoadingModels(true);
      setModelFailure(null);
      try {
        const uniqueModels = await fetchAvailableModels(effectiveApiKey, true);
        if (!active) return;
        const nextOptions = Array.from(new Set(uniqueModels.map((model) => model.model_group)));
        setModelOptions(nextOptions);
        setModelInfo(uniqueModels);
      } catch (error) {
        console.error("CompareUI: failed to fetch models", error);
        if (active) {
          setModelFailure(diagnoseRequest(error));
          setModelOptions([]);
          setModelInfo([]);
        }
      } finally {
        if (active) {
          setIsLoadingModels(false);
        }
      }
    };
    loadModels();
    return () => {
      active = false;
    };
  }, [effectiveApiKey]);

  // Fetch agents when A2A mode is selected
  useEffect(() => {
    let active = true;
    const loadAgents = async () => {
      if (!effectiveApiKey || !isA2AMode) {
        setAgentOptions([]);
        return;
      }
      setIsLoadingAgents(true);
      try {
        const agents = await fetchAvailableAgents(effectiveApiKey, customProxyBaseUrl || undefined);
        if (!active) return;
        setAgentOptions(agents);
      } catch (error) {
        console.error("CompareUI: failed to fetch agents", error);
        if (active) {
          setAgentOptions([]);
        }
      } finally {
        if (active) {
          setIsLoadingAgents(false);
        }
      }
    };
    loadAgents();
    return () => {
      active = false;
    };
  }, [effectiveApiKey, isA2AMode]);

  useEffect(() => {
    if (modelOptions.length === 0) {
      return;
    }
    setComparisons((prev) =>
      prev.map((comparison, index) => {
        return {
          ...comparison,
          temperature: comparison.temperature ?? 1,
          maxTokens: comparison.maxTokens ?? 2048,
          applyAcrossModels: comparison.applyAcrossModels ?? false,
          useAdvancedParams: comparison.useAdvancedParams ?? false,
          ...(comparison.model
            ? {}
            : {
                model: modelOptions[index % modelOptions.length] ?? "",
              }),
        };
      }),
    );
  }, [modelOptions]);
  useEffect(() => {
    setComparisons((previous) =>
      previous.map((comparison) => {
        const model = modelInfo.find((item) => item.model_group === comparison.model);
        const endpoints = textEndpoints(model);
        const endpoint = selectEndpoint(model ? { ...model, endpoints } : undefined, comparison.endpoint ?? null);
        return comparison.endpoint === endpoint ? comparison : { ...comparison, endpoint };
      }),
    );
  }, [modelInfo, comparisons.map((comparison) => comparison.model).join("\n")]);
  const maxComparisons = 3;
  /** addComparison 创建独立对比卡片，最多三张。参数：无；返回：无；新卡片等待选择模型及其端点。 */
  const addComparison = () => {
    if (comparisons.length >= maxComparisons) {
      return;
    }
    const fallbackModel = modelOptions[comparisons.length % (modelOptions.length || 1)] ?? "";
    const fallbackAgent = agentOptions[comparisons.length % (agentOptions.length || 1)]?.agent_name ?? "";
    const newComparison: ComparisonInstance = {
      id: Date.now().toString(),
      model: fallbackModel,
      agent: fallbackAgent,
      messages: [],
      isLoading: false,
      tags: [],
      mcpTools: [],
      vectorStores: [],
      guardrails: [],
      temperature: 1,
      maxTokens: 2048,
      applyAcrossModels: false,
      useAdvancedParams: false,
    };
    setComparisons((prev) => [...prev, newComparison]);
  };
  /** removeComparison 删除指定卡片并保留至少一张。参数 id：卡片身份；返回：无。 */
  const removeComparison = (id: string) => {
    if (comparisons.length > 1) {
      stopRequests(id);
      setComparisons((prev) => {
        const next = prev.filter((c) => c.id !== id);
        return next;
      });
    }
  };
  type UpdateOptions = {
    applyToAll?: boolean;
    keysToApply?: (keyof ComparisonInstance)[];
  };
  /** updateComparison 更新卡片状态，可显式共享指定生成设置。
   * 参数 id：卡片身份；updates：字段变更；options：共享范围；返回：无。模型和端点选择保持独立。 */
  const updateComparison = (id: string, updates: Partial<ComparisonInstance>, options?: UpdateOptions) => {
    if (updates.model !== undefined || updates.endpoint !== undefined || updates.agent !== undefined) {
      stopRequests(id);
      updates = { ...updates, messages: [], traceId: undefined, failure: null, isLoading: false };
    }
    setComparisons((prev) => {
      if (options?.applyToAll && options.keysToApply?.length) {
        const sharedUpdates: Partial<ComparisonInstance> = {};
        options.keysToApply.forEach((key) => {
          const value = updates[key];
          if (value !== undefined) {
            sharedUpdates[key] = Array.isArray(value) ? ([...value] as any) : (value as any);
          }
        });
        const hasSharedUpdates = Object.keys(sharedUpdates).length > 0;
        return prev.map((comparison) => {
          if (comparison.id === id) {
            return {
              ...comparison,
              ...updates,
            };
          }
          if (!hasSharedUpdates) {
            return comparison;
          }
          return {
            ...comparison,
            ...sharedUpdates,
          };
        });
      }
      return prev.map((comparison) =>
        comparison.id === id
          ? {
              ...comparison,
              ...updates,
            }
          : comparison,
      );
    });
  };
  /** handleFileUpload 保存待发送附件与预览地址。参数 file：用户文件；返回 false，阻止自动上传。 */
  const handleFileUpload = (file: File): false => {
    if (uploadedFilePreviewUrl) {
      URL.revokeObjectURL(uploadedFilePreviewUrl);
    }
    setUploadedFile(file);
    setUploadedFilePreviewUrl(URL.createObjectURL(file));
    return false;
  };
  /** handleRemoveFile 释放附件预览地址并清除附件。参数：无；返回：无。 */
  const handleRemoveFile = () => {
    if (uploadedFilePreviewUrl) {
      URL.revokeObjectURL(uploadedFilePreviewUrl);
    }
    setUploadedFile(null);
    setUploadedFilePreviewUrl(null);
  };
  /** clearAllChats 清空所有卡片对话和指标，保留当前模型与端点设置。参数：无；返回：无。 */
  const clearAllChats = () => {
    stopRequests();
    setComparisons((prev) =>
      prev.map((comparison) => ({
        ...comparison,
        failure: null,
        messages: [],
        traceId: undefined,
        isLoading: false,
      })),
    );
    setInputValue("");
    handleRemoveFile();
  };
  /** appendAssistantChunk 将文本增量写入指定卡片的最后一条助手消息。
   * 参数 comparisonId：卡片身份；chunk：文本增量；model：实际回答模型；返回：无。 */
  const appendAssistantChunk = (comparisonId: string, chunk: string, model?: string) => {
    if (!chunk) {
      return;
    }
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          const existingContent = typeof last.content === "string" ? last.content : "";
          messages[messages.length - 1] = {
            ...last,
            content: existingContent + chunk,
            model: last.model ?? model,
          };
        } else {
          messages.push({
            role: "assistant",
            content: chunk,
            model,
          });
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  /** appendReasoningContent 累加指定卡片的推理增量。参数 comparisonId/chunk：卡片身份和文本；返回：无。 */
  const appendReasoningContent = (comparisonId: string, chunk: string) => {
    if (!chunk) {
      return;
    }
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          messages[messages.length - 1] = {
            ...last,
            reasoningContent: (last.reasoningContent || "") + chunk,
          };
        } else if (last && last.role === "user") {
          messages.push({
            role: "assistant",
            content: "",
            reasoningContent: chunk,
          });
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  /** updateTimingDataForComparison 写入本次回答的首字延迟。参数 comparisonId：卡片身份；timeToFirstToken：秒；返回：无。 */
  const updateTimingDataForComparison = (comparisonId: string, timeToFirstToken: number) => {
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          messages[messages.length - 1] = {
            ...last,
            timeToFirstToken,
          };
        } else if (last && last.role === "user") {
          messages.push({
            role: "assistant",
            content: "",
            timeToFirstToken,
          });
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  /** updateTotalLatencyForComparison 写入本次回答的总延迟。参数 comparisonId：卡片身份；totalLatency：秒；返回：无。 */
  const updateTotalLatencyForComparison = (comparisonId: string, totalLatency: number) => {
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          messages[messages.length - 1] = {
            ...last,
            totalLatency,
          };
        } else if (last && last.role === "user") {
          messages.push({
            role: "assistant",
            content: "",
            totalLatency,
          });
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  /** updateUsageDataForComparison 写入供应商报告的规范用量，不在浏览器估算费用。
   * 参数 comparisonId：卡片身份；usage：实测 token 等事实；toolName：可选工具标识；返回：无。 */
  const updateUsageDataForComparison = (comparisonId: string, usage: TokenUsage, toolName?: string) => {
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          messages[messages.length - 1] = {
            ...last,
            usage,
            toolName,
          };
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  /** updateSearchResultsForComparison 保存指定卡片的检索结果。参数 comparisonId/searchResults：身份和结果列表；返回：无。 */
  const updateSearchResultsForComparison = (comparisonId: string, searchResults: VectorStoreSearchResponse[]) => {
    if (!searchResults) {
      return;
    }
    setComparisons((prev) =>
      prev.map((comparison) => {
        if (comparison.id !== comparisonId) {
          return comparison;
        }
        const messages = [...comparison.messages];
        const last = messages[messages.length - 1];
        if (last && last.role === "assistant") {
          messages[messages.length - 1] = {
            ...last,
            searchResults,
          };
        }
        return {
          ...comparison,
          messages,
        };
      }),
    );
  };
  const canUseSessionKey = Boolean(accessToken);
  /** handleSendMessage 校验每张卡片所选模型的真实文本端点，再并发发送同一输入。
   * 参数 input：用户输入；返回 Promise<void>；各请求独立更新文本、用量和错误状态。
   * adapted 聊天保留原有富交互；其他协议使用真实公开路径，原生协议不混入网关专属字段。 */
  const handleSendMessage = async (input: string) => {
    if (preparing.current || requests.current.size > 0) return;
    const trimmed = input.trim();
    const hasAttachment = Boolean(uploadedFile);
    if (!trimmed && !hasAttachment) {
      return;
    }
    if (!effectiveApiKey) {
      toast.fromError(t("Please provide a Virtual Key or select Current UI Session"));
      return;
    }
    const priorRound = retryRound.current;
    const round =
      priorRound?.text === trimmed && priorRound.file === uploadedFile
        ? priorRound
        : { text: trimmed, file: uploadedFile, completed: new Set<string>() };
    const targetComparisons = comparisons.filter((card) => !round.completed.has(card.id));
    if (targetComparisons.length === 0) {
      return;
    }
    // Validate selection based on endpoint type
    if (targetComparisons.some((comparison) => !hasValidSelection(comparison, selectedEndpoint))) {
      toast.fromError(endpointConfig.validationMessage);
      return;
    }

    if (
      !isA2AMode &&
      targetComparisons.some(
        (comparison) =>
          !textEndpoints(modelInfo.find((model) => model.model_group === comparison.model)).some(
            (endpoint) => endpoint.path === comparison.endpoint,
          ),
      )
    ) {
      toast.fromError("请先为每个模型选择支持的对话端点");
      return;
    }
    preparing.current = true;
    const preparationGeneration = generation.current;
    let apiUserMessage;
    try {
      apiUserMessage = hasAttachment
        ? await createChatMultimodalMessage(trimmed, uploadedFile as File)
        : { role: "user", content: trimmed };
    } catch (error) {
      toast.fromError(error);
      return;
    } finally {
      preparing.current = false;
    }
    // 附件解析期间切换上下文或删除卡片，整批准备作废，不能向旧模型发起请求。
    if (preparationGeneration !== generation.current) return;
    retryRound.current = round;
    const displayUserMessage = createChatDisplayMessage(
      trimmed,
      hasAttachment,
      uploadedFilePreviewUrl || undefined,
      uploadedFile?.name,
    );

    const preparedTargets = new Map<
      string,
      {
        id: string;
        model: string;
        agent: string;
        inputMessage: string;
        traceId: string;
        tags: string[];
        vectorStores: string[];
        guardrails: string[];
        temperature: number;
        maxTokens: number;
        displayMessages: MessageType[];
        apiChatHistory: Array<{ role: string; content: string | any[] }>;
      }
    >();
    targetComparisons.forEach((comparison) => {
      const traceId = comparison.traceId ?? uuidv4();
      const apiChatHistory = [
        ...comparison.messages.map(({ role, content }) => ({
          role,
          content: Array.isArray(content) ? content : typeof content === "string" ? content : "",
        })),
        apiUserMessage,
      ];
      preparedTargets.set(comparison.id, {
        id: comparison.id,
        model: comparison.model,
        agent: comparison.agent,
        inputMessage: trimmed,
        traceId,
        tags: comparison.tags,
        vectorStores: comparison.vectorStores,
        guardrails: comparison.guardrails,
        temperature: comparison.temperature,
        maxTokens: comparison.maxTokens,
        displayMessages: [...comparison.messages, displayUserMessage],
        apiChatHistory,
      });
    });
    if (preparedTargets.size === 0) {
      return;
    }
    setComparisons((prev) =>
      prev.map((comparison) => {
        const prepared = preparedTargets.get(comparison.id);
        if (!prepared) {
          return comparison;
        }
        return {
          ...comparison,
          traceId: prepared.traceId,
          messages: prepared.displayMessages,
          isLoading: true,
          failure: null,
        };
      }),
    );
    const responseUpdates = {
      appendAssistantChunk,
      appendReasoningContent,
      updateTimingDataForComparison,
      updateUsageDataForComparison,
      updateSearchResultsForComparison,
      updateTotalLatencyForComparison,
    };
    const outcomes = await Promise.all(
      Array.from(preparedTargets.values()).map(async (prepared) => {
        const controller = new AbortController();
        requests.current.set(prepared.id, controller);
        requestHistories.current.set(prepared.id, comparisons.find((card) => card.id === prepared.id)?.messages ?? []);
        const active = () => requests.current.get(prepared.id) === controller && !controller.signal.aborted;
        const {
          appendAssistantChunk,
          appendReasoningContent,
          updateTimingDataForComparison,
          updateUsageDataForComparison,
          updateSearchResultsForComparison,
          updateTotalLatencyForComparison,
        } = guardCallbacks(responseUpdates, active);
        const tags = prepared.tags.length > 0 ? prepared.tags : undefined;
        const vectorStoreIds = prepared.vectorStores.length > 0 ? prepared.vectorStores : undefined;
        const guardrails = prepared.guardrails.length > 0 ? prepared.guardrails : undefined;
        const comparison = comparisons.find((c) => c.id === prepared.id);
        const useAdvancedParams = comparison?.useAdvancedParams ?? false;

        // Use A2A or chat completion based on endpoint
        try {
          const requestPromise = isA2AMode
            ? makeA2AStreamMessageRequest(
                prepared.agent,
                prepared.inputMessage,
                (text, model) => {
                  if (!active()) return;
                  // A2A sends full accumulated text, so replace instead of append
                  setComparisons((prev) =>
                    prev.map((c) => {
                      if (c.id !== prepared.id) return c;
                      const messages = [...c.messages];
                      const last = messages[messages.length - 1];
                      if (last && last.role === "assistant") {
                        messages[messages.length - 1] = { ...last, content: text, model: last.model ?? model };
                      } else {
                        messages.push({ role: "assistant", content: text, model });
                      }
                      return { ...c, messages };
                    }),
                  );
                },
                effectiveApiKey,
                controller.signal,
                (time) => updateTimingDataForComparison(prepared.id, time),
                (latency) => updateTotalLatencyForComparison(prepared.id, latency),
                undefined, // onA2AMetadata
                customProxyBaseUrl || undefined,
              )
            : comparison?.endpoint === "/v1/chat/completions"
              ? makeOpenAIChatCompletionRequest(
                  prepared.apiChatHistory,
                  (chunk, model) => appendAssistantChunk(prepared.id, chunk, model),
                  prepared.model,
                  effectiveApiKey,
                  tags,
                  controller.signal,
                  (content) => appendReasoningContent(prepared.id, content),
                  (time) => updateTimingDataForComparison(prepared.id, time),
                  (usage) => updateUsageDataForComparison(prepared.id, usage),
                  prepared.traceId,
                  vectorStoreIds,
                  guardrails,
                  undefined,
                  undefined,
                  undefined,
                  (results) => updateSearchResultsForComparison(prepared.id, results),
                  useAdvancedParams ? prepared.temperature : undefined,
                  useAdvancedParams ? prepared.maxTokens : undefined,
                  (latency) => updateTotalLatencyForComparison(prepared.id, latency),
                  customProxyBaseUrl || undefined,
                )
              : callTextEndpoint({
                  endpoint: textEndpoints(modelInfo.find((model) => model.model_group === prepared.model)).find(
                    (endpoint) => endpoint.path === comparison?.endpoint,
                  )!,
                  base: customProxyBaseUrl || getProxyBaseUrl(),
                  signal: controller.signal,
                  key: effectiveApiKey,
                  model: prepared.model,
                  messages: prepared.apiChatHistory,
                  onText: (chunk) => appendAssistantChunk(prepared.id, chunk, prepared.model),
                  tags,
                  onUsage: (usage) => updateUsageDataForComparison(prepared.id, usage),
                  onTiming: (time) => updateTimingDataForComparison(prepared.id, time),
                  onLatency: (time) => updateTotalLatencyForComparison(prepared.id, time),
                  temperature: useAdvancedParams ? prepared.temperature : undefined,
                  maxTokens: useAdvancedParams ? prepared.maxTokens : undefined,
                });

          await requestPromise;
          if (active()) round.completed.add(prepared.id);
          return active();
        } catch (error) {
          if (active())
            setComparisons((previous) =>
              previous.map((card) =>
                card.id === prepared.id
                  ? { ...card, messages: comparison?.messages ?? [], failure: diagnoseRequest(error) }
                  : card,
              ),
            );
          return false;
        } finally {
          if (requests.current.get(prepared.id) === controller) {
            requests.current.delete(prepared.id);
            requestHistories.current.delete(prepared.id);
            setComparisons((previous) =>
              previous.map((card) => (card.id === prepared.id ? { ...card, isLoading: false } : card)),
            );
          }
        }
      }),
    );
    if (preparationGeneration === generation.current && outcomes.every(Boolean)) {
      retryRound.current = null;
      setInputValue("");
      handleRemoveFile();
    }
  };

  /** handleInputChange 更新公共输入。参数 value：文本；返回：无。 */
  const handleInputChange = (value: string) => {
    setInputValue(value);
  };
  /** handleSubmit 提交当前公共输入。参数：无；返回：无。 */
  const handleSubmit = () => {
    void handleSendMessage(inputValue);
  };
  /** handleFollowUpSelect 将选中的追问发送到各卡片。参数 question：追问文本；返回：无。 */
  const handleFollowUpSelect = (question: string) => {
    setInputValue(question);
  };
  const hasMessages = comparisons.some((comparison) => comparison.messages.length > 0);
  const isAnyComparisonLoading = comparisons.some((comparison) => comparison.isLoading);
  const hasAttachment = Boolean(uploadedFile);
  const isUploadedFilePdf = Boolean(uploadedFile?.name.toLowerCase().endsWith(".pdf"));
  const showSuggestedPrompts = !hasMessages && !isAnyComparisonLoading && !hasAttachment;
  return (
    <div className="w-full h-full min-h-0 p-3 bg-card">
      <div className="rounded-xl border border-border bg-card h-full min-h-0 overflow-hidden flex flex-col">
        {modelFailure && <RequestDiagnostic failure={modelFailure} />}
        <div className="border-b px-4 py-2">
          {isAnyComparisonLoading && (
            <Button variant="outline" size="sm" onClick={() => stopRequests()}>
              停止全部
            </Button>
          )}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-muted-foreground">{t("Virtual Key Source")}</span>
              <Select
                value={apiKeySource}
                onValueChange={(value) => setApiKeySource(value as "session" | "custom")}
                disabled={disabledPersonalKeyCreation}
              >
                <SelectTrigger className="w-48" aria-label={t("Virtual Key Source")}>
                  <SelectValue>{apiKeySource === "custom" ? t("Virtual Key") : t("Current UI Session")}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="session" disabled={!canUseSessionKey}>
                    {t("Current UI Session")}
                  </SelectItem>
                  <SelectItem value="custom">{t("Virtual Key")}</SelectItem>
                </SelectContent>
              </Select>
              {apiKeySource === "custom" && (
                <Input
                  type="password"
                  value={customApiKey}
                  onChange={(event) => setCustomApiKey(event.target.value)}
                  placeholder={t("Enter Virtual Key")}
                  className="w-56"
                />
              )}
            </div>
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-muted-foreground">{t("Comparison mode")}</span>
              <Select value={selectedEndpoint} onValueChange={(value) => setSelectedEndpoint(value as EndpointIdType)}>
                <SelectTrigger className="w-56" aria-label={t("Comparison mode")}>
                  <SelectValue>{t(isA2AMode ? "Agents" : "Models")}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {getAvailableEndpoints().map((endpoint) => (
                    <SelectItem key={endpoint.value} value={endpoint.value}>
                      {t(endpoint.value === EndpointId.A2A_AGENTS ? "Agents" : "Models")}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <Button
                variant="outline"
                onClick={clearAllChats}
                disabled={!hasMessages && !comparisons.some((card) => card.failure)}
              >
                <Eraser />
                {t("Clear All Chats")}
              </Button>
              <Tooltip>
                <TooltipTrigger render={<span className="inline-flex" />}>
                  <Button variant="outline" onClick={addComparison} disabled={comparisons.length >= maxComparisons}>
                    <Plus />
                    {t("Add Comparison")}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {comparisons.length >= maxComparisons
                    ? t("Compare up to 3 models at a time")
                    : t("Add another comparison")}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>
        </div>

        <div
          className={
            "grid flex-1 min-h-0 overflow-auto " +
            (comparisons.length === 3 ? "xl:grid-cols-3" : comparisons.length === 2 ? "lg:grid-cols-2" : "grid-cols-1")
          }
        >
          {comparisons.map((comparison) => (
            <ComparisonPanel
              key={comparison.id}
              comparison={comparison}
              onUpdate={(updates, options) =>
                updateComparison(
                  comparison.id,
                  updates.model !== undefined ? { ...updates, endpoint: null } : updates,
                  options,
                )
              }
              onRemove={() => removeComparison(comparison.id)}
              canRemove={comparisons.length > 1}
              selectorOptions={selectorOptions}
              isLoadingOptions={isLoadingOptions}
              endpointConfig={endpointConfig}
              apiKey={effectiveApiKey}
              unavailableReason={modelInfo.find((model) => model.model_group === comparison.model)?.unavailable_reason}
              supportsGatewaySettings={comparison.endpoint === "/v1/chat/completions"}
              endpointOptions={textEndpoints(modelInfo.find((model) => model.model_group === comparison.model)).map(
                (endpoint) => ({ value: endpoint.path, label: endpointLabel(endpoint) }),
              )}
            />
          ))}
        </div>
        <div className="flex shrink-0 justify-center border-t bg-card py-3">
          <div className="w-full max-w-3xl px-4">
            <div className="border border-border shadow-lg rounded-xl bg-card p-4">
              <div className="flex items-center justify-between gap-4 mb-3 min-h-8">
                {hasAttachment ? (
                  <span className="text-sm text-muted-foreground">{t("Attachment ready to send")}</span>
                ) : showSuggestedPrompts ? (
                  <div className="flex items-center gap-2 overflow-x-auto">
                    {SUGGESTED_PROMPTS.map((prompt) => (
                      <button
                        key={prompt}
                        type="button"
                        onClick={() => handleFollowUpSelect(t(prompt))}
                        className="shrink-0 rounded-full border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent cursor-pointer"
                      >
                        {t(prompt)}
                      </button>
                    ))}
                  </div>
                ) : haveAllResponses && !hasAttachment ? (
                  <div className="flex items-center gap-2 overflow-x-auto">
                    {GENERIC_FOLLOW_UPS.map((question) => (
                      <button
                        key={question}
                        type="button"
                        onClick={() => handleFollowUpSelect(t(question))}
                        className="shrink-0 rounded-full border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent cursor-pointer"
                      >
                        {t(question)}
                      </button>
                    ))}
                  </div>
                ) : isAnyComparisonLoading ? (
                  <span className="flex items-center gap-2 text-sm text-muted-foreground">
                    <span className="h-2 w-2 rounded-full bg-info animate-pulse" aria-hidden />
                    {endpointConfig.loadingMessage}
                  </span>
                ) : (
                  <span className="text-sm text-muted-foreground">{endpointConfig.inputPlaceholder}</span>
                )}
              </div>
              {uploadedFile && (
                <div className="mb-3">
                  <div className="flex items-center gap-3 p-3 bg-muted rounded-lg border border-border">
                    <div className="relative inline-block">
                      {isUploadedFilePdf ? (
                        <div className="w-10 h-10 rounded-md bg-destructive flex items-center justify-center text-destructive-foreground">
                          <FileText className="size-4" aria-label="file-pdf" />
                        </div>
                      ) : (
                        <img
                          src={uploadedFilePreviewUrl || ""}
                          alt={t("Upload preview")}
                          className="w-10 h-10 rounded-md border border-border object-cover"
                        />
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-foreground truncate">{uploadedFile.name}</div>
                      <div className="text-xs text-muted-foreground">{isUploadedFilePdf ? "PDF" : t("Image")}</div>
                    </div>
                    <button
                      className="flex items-center justify-center w-6 h-6 text-muted-foreground hover:text-foreground hover:bg-accent rounded-full transition-colors"
                      onClick={handleRemoveFile}
                      aria-label={t("Remove attachment")}
                    >
                      <Trash2 className="size-3" />
                    </button>
                  </div>
                </div>
              )}
              <MessageInput
                value={inputValue}
                onChange={handleInputChange}
                onSend={handleSubmit}
                disabled={comparisons.length === 0 || comparisons.some((comparison) => comparison.isLoading)}
                hasAttachment={hasAttachment}
                uploadComponent={
                  <ChatImageUpload
                    chatUploadedImage={uploadedFile}
                    chatImagePreviewUrl={uploadedFilePreviewUrl}
                    onImageUpload={handleFileUpload}
                    onRemoveImage={handleRemoveFile}
                  />
                }
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
