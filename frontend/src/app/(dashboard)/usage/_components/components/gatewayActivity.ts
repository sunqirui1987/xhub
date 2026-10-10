/** 网关请求统计来自已持久化账单，与费用及 Token 使用同一归属范围；页面仅在管理员未筛选用户时使用此汇总。 */

export const GATEWAY_TOP_ROUTES = 15;

export interface GatewayActivity {
  total_successful_requests: number;
  total_failed_requests: number;
  by_date: { date: string; successful_requests: number; failed_requests: number }[];
  by_route: { category: string; route: string; successful_requests: number; failed_requests: number }[];
}

/** A fetched result carrying the range key it was fetched for. */
export interface FetchedForRange<T> {
  rangeKey: string;
  value: T;
}

export type FetchedGatewayActivity = FetchedForRange<GatewayActivity>;

/** Extends Record so it satisfies the chart component's row constraint. */
export interface GatewayRouteBar extends Record<string, unknown> {
  route: string;
  successful_requests: number;
  failed_requests: number;
}

/**
 * Identifies what a result was fetched for: the date range, plus any other
 * input that changes the answer. The usage aggregate is scoped to a user, so
 * two results covering the same dates still describe different numbers.
 */
export const fetchedRangeKey = (
  startTime: Date | null | undefined,
  endTime: Date | null | undefined,
  scope: string | null | undefined = null,
): string => `${startTime?.toISOString() ?? ""}|${endTime?.toISOString() ?? ""}|${scope ?? ""}`;

/**
 * The value safe to render right now, or null to fall back.
 *
 * Clearing the state inside the fetch effect is one render too late: the render
 * that follows a date change still holds the previous range's value and can
 * paint before effects run. Comparing the stamp during render is what makes a
 * superseded range unrepresentable rather than merely brief.
 */
export const selectForRange = <T>(fetched: FetchedForRange<T> | null, currentRangeKey: string): T | null =>
  fetched != null && fetched.rangeKey === currentRangeKey ? fetched.value : null;

/**
 * As `selectForRange`, and additionally withholds the counts from a non-admin:
 * 页面仅在管理员全局视图使用该源；其他视图使用各自筛选后的账单汇总。
 */
export const selectGatewayActivity = (
  isAdmin: boolean,
  fetched: FetchedGatewayActivity | null,
  currentRangeKey: string,
): GatewayActivity | null => (isAdmin ? selectForRange(fetched, currentRangeKey) : null);

/**
 * Bars for the endpoint breakdown chart, capped so a deployment exercising many
 * endpoints does not render an unreadable axis. `by_route` arrives sorted by
 * successful_requests descending, so the cap keeps the busiest endpoints.
 */
export const topGatewayRoutes = (
  activity: GatewayActivity | null,
  limit: number = GATEWAY_TOP_ROUTES,
): GatewayRouteBar[] =>
  (activity?.by_route ?? []).slice(0, limit).map((entry) => ({
    // The llm routes are already fully qualified; mcp and a2a routes are not, so
    // their category prefix is what keeps "/mcp" apart from "/a2a".
    route: entry.category === "llm" ? entry.route : `${entry.category}${entry.route}`,
    successful_requests: entry.successful_requests,
    failed_requests: entry.failed_requests,
  }));
