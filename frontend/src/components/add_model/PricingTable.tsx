import { pricingRates, type CatalogRow } from "./modelEditorPricing";

const units: Record<string, string> = { token: "百万 Token", picture: "张", second: "秒", query: "次" };
const sides: Record<string, string> = {
  input: "输入",
  output: "输出",
  cache_read: "缓存读取",
  cache_write: "缓存写入",
  batch_input: "批量输入",
  batch_output: "批量输出",
};
const windows: Record<string, string> = { all: "全天", peak: "高峰", offpeak: "闲时" };

export default function PricingTable({ row }: { row?: CatalogRow }) {
  const rates = pricingRates(row);
  if (!rates.length)
    return (
      <p className="rounded-lg border border-dashed p-5 text-sm text-muted-foreground">
        尚未设置价格。请在编辑中选择定价模型或填写单价。
      </p>
    );
  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-left text-sm">
        <thead className="bg-muted/50">
          <tr>
            <th className="p-3 font-medium">计费项目</th>
            <th className="p-3 font-medium">时段</th>
            <th className="p-3 text-right font-medium">单价（USD）</th>
          </tr>
        </thead>
        <tbody>
          {rates.map((rate, index) => (
            <tr key={index} className="border-t">
              <td className="p-3">
                {sides[rate.side] ?? rate.side}
                {rate.variant && rate.variant !== "uncached" ? " · " + rate.variant : ""}
              </td>
              <td className="p-3 text-muted-foreground">{windows[rate.window] ?? rate.window}</td>
              <td className="whitespace-nowrap p-3 text-right tabular-nums">
                {Number((rate.usd * (rate.measure === "token" ? 1e6 : 1)).toPrecision(12))} /{" "}
                {units[rate.measure] ?? rate.measure}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
