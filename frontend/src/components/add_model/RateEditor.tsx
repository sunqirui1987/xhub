import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { PriceRate } from "./modelEditorPricing";
const measures = { token: "百万 Token", picture: "张", second: "秒", query: "次" };
const sides = {
  input: "输入",
  output: "输出",
  cache_read: "缓存读取",
  cache_write: "缓存写入",
  batch_input: "批量输入",
  batch_output: "批量输出",
};
const windows = { all: "全天", offpeak: "闲时", peak: "高峰" };
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
export default function RateEditor({
  rates,
  onChange,
}: {
  rates: PriceRate[];
  onChange: (rates: PriceRate[]) => void;
}) {
  const update = (index: number, patch: Partial<PriceRate>) =>
    onChange(rates.map((rate, position) => (position === index ? { ...rate, ...patch } : rate)));
  return (
    <div className="space-y-4">
      <p className="text-xs leading-5 text-muted-foreground">
        可同时设置 Token、图片、秒数与缓存分时价。规格使用目录中的原有标识，例如分辨率或思考模式；留空表示通用费率。
      </p>
      {rates.map((rate, index) => (
        <fieldset key={index} className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2">
          <label className="space-y-1 text-xs">
            计费单位
            <select
              aria-label={"费率 " + (index + 1) + " 单位"}
              className={selectClass}
              value={rate.measure}
              onChange={(event) => update(index, { measure: event.target.value })}
            >
              {Object.entries(measures).map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-1 text-xs">
            计费项目
            <select
              aria-label={"费率 " + (index + 1) + " 项目"}
              className={selectClass}
              value={rate.side}
              onChange={(event) => update(index, { side: event.target.value })}
            >
              {Object.entries(sides).map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-1 text-xs">
            计费时段
            <select
              aria-label={"费率 " + (index + 1) + " 时段"}
              className={selectClass}
              value={rate.window}
              onChange={(event) => update(index, { window: event.target.value })}
            >
              {Object.entries(windows).map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-1 text-xs">
            单价（USD / {measures[rate.measure as keyof typeof measures]}）
            <Input
              aria-label={"费率 " + (index + 1) + " 单价"}
              type="number"
              min="0"
              step="any"
              value={
                Number.isFinite(rate.usd)
                  ? Number((rate.usd * (rate.measure === "token" ? 1e6 : 1)).toPrecision(12))
                  : ""
              }
              onChange={(event) =>
                update(index, {
                  usd:
                    event.target.value === "" ? NaN : Number(event.target.value) / (rate.measure === "token" ? 1e6 : 1),
                })
              }
            />
          </label>
          <label className="space-y-1 text-xs">
            规格标识
            <Input
              aria-label={"费率 " + (index + 1) + " 规格"}
              value={rate.variant ?? ""}
              onChange={(event) => update(index, { variant: event.target.value })}
              placeholder="通用费率"
            />
          </label>
          <Button
            type="button"
            variant="ghost"
            className="self-end text-destructive"
            onClick={() => onChange(rates.filter((_, position) => index !== position))}
          >
            移除此费率
          </Button>
        </fieldset>
      ))}
      <Button
        type="button"
        variant="outline"
        onClick={() => onChange([...rates, { measure: "token", side: "input", window: "all", usd: NaN }])}
      >
        添加费率
      </Button>
    </div>
  );
}
