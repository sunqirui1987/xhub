import { ModelGroup } from "@/components/llm_calls/fetch_models";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";
import { endpointUIType, modelEndpoints } from "@/components/llm_calls/model_endpoints";
/** determineEndpointType 返回模型首个真实绑定对应的表单类型。
 * 参数 selectedModel：对外模型名；modelInfo：已授权模型列表。返回：表单类型或 null。
 * 找不到模型、未声明绑定或只能使用原生编辑器时返回 null，不从模型分类推断。
 * 调用：对话端点工具消费者。测试：EndpointUtils.test.tsx。
 */
export const determineEndpointType = (selectedModel: string, modelInfo: ModelGroup[]): EndpointType | null =>
  endpointUIType(modelEndpoints(modelInfo.find((model) => model.model_group === selectedModel))[0]);
/** isModelCompatibleWithEndpoint 校验模型明确支持的表单类型或原生路径。
 * 参数 model：模型元数据；endpointType：表单类型或路径。返回：存在匹配绑定时为 true。
 * 表单类型只用于界面分类；实际调用仍需选中 ModelEndpoint，保留原始协议与路径。
 * 调用：filterModelsForEndpoint。测试：EndpointUtils.test.tsx。
 */
export const isModelCompatibleWithEndpoint = (model: ModelGroup, endpointType: EndpointType | string): boolean =>
  modelEndpoints(model).some((endpoint) => (endpointUIType(endpoint) ?? endpoint.path) === endpointType);
/** filterModelsForEndpoint 按真实端点绑定过滤模型。
 * 参数 models：已授权模型列表；endpointType：目标表单类型。返回：保留输入顺序的子集。
 * 不扩大模型权限，也不为缺少绑定的模型补建默认 chat 端点。
 * 调用：需要按表单能力展示模型的界面。测试：EndpointUtils.test.tsx。
 */
export const filterModelsForEndpoint = (models: ModelGroup[], endpointType: EndpointType): ModelGroup[] =>
  models.filter((model) => isModelCompatibleWithEndpoint(model, endpointType));
