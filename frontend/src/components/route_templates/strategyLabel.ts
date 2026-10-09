import { formatStrategyLabel } from "@/components/routing_groups/strategy";

/** Describe this gateway's preserved default behavior accurately. */
export const formatTemplateStrategyLabel = (strategy: string): string =>
  strategy === "simple-shuffle" ? "pages.routeTemplates.priorityStrategy" : formatStrategyLabel(strategy);
