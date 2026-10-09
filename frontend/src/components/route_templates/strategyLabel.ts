import { formatStrategyLabel } from "@/components/routing_groups/strategy";

/** formatTemplateStrategyLabel 将策略标识转换为翻译键；参数为策略字符串，返回展示键，供模板库调用，无副作用。 */
export const formatTemplateStrategyLabel = (strategy: string): string =>
  strategy === "simple-shuffle" ? "pages.routeTemplates.priorityStrategy" : formatStrategyLabel(strategy);
