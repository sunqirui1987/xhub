const STRATEGY_LABELS: Readonly<Record<string, string>> = {
  "simple-shuffle": "Simple Shuffle",
  "least-busy": "Least Busy",
  "usage-based-routing": "Usage Based",
  "usage-based-routing-v2": "Usage Based v2",
  "latency-based-routing": "Latency Based",
  "cost-based-routing": "Cost Based",
};

export const formatStrategyLabel = (strategy: string): string => STRATEGY_LABELS[strategy] ?? strategy;
