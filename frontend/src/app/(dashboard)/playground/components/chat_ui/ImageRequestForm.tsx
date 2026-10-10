import { useT } from "@/i18n";
import { Input } from "@/components/ui/input";

/** 图片调试参数表单；doc 为原生草稿，disabled 为请求中状态，onChange 接收字段及新值。
 * 返回提示词、尺寸、数量和可选质量控件；由原生调试台调用，修改同步回 JSON 且保留扩展参数。
 * 空质量删除字段，其他空值保留供提交校验；不按模型名称推断渠道支持能力。 */
export function ImageRequestForm({
  doc,
  disabled,
  onChange,
}: {
  doc: Record<string, unknown>;
  disabled: boolean;
  onChange: (key: string, value: unknown) => void;
}) {
  const t = useT();
  return (
    <div className="space-y-4">
      <label className="block space-y-2 text-sm">
        <span className="font-medium">{t("myModels.imagePrompt")}</span>
        <textarea
          aria-label={t("myModels.imagePrompt")}
          placeholder={t("myModels.imagePromptPlaceholder")}
          disabled={disabled}
          className="min-h-32 w-full resize-y rounded-lg border border-border bg-card p-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={typeof doc.prompt === "string" ? doc.prompt : ""}
          onChange={(event) => onChange("prompt", event.target.value)}
        />
      </label>
      <div className="grid grid-cols-2 gap-3">
        <label className="space-y-2 text-sm">
          <span className="block font-medium">{t("myModels.imageSize")}</span>
          <Input
            aria-label={t("myModels.imageSize")}
            list="image-sizes"
            placeholder="1024x1024"
            disabled={disabled}
            value={typeof doc.size === "string" ? doc.size : ""}
            onChange={(event) => onChange("size", event.target.value)}
          />
          <datalist id="image-sizes">
            {["1024x1024", "1536x1024", "1024x1536", "auto"].map((size) => (
              <option key={size} value={size} />
            ))}
          </datalist>
        </label>
        <label className="space-y-2 text-sm">
          <span className="block font-medium">{t("myModels.imageCount")}</span>
          <Input
            aria-label={t("myModels.imageCount")}
            type="number"
            min={1}
            step={1}
            disabled={disabled}
            value={typeof doc.n === "number" ? doc.n : ""}
            onChange={(event) => onChange("n", event.target.value === "" ? "" : Number(event.target.value))}
          />
        </label>
      </div>
      <label className="block space-y-2 text-sm">
        <span className="font-medium">{t("myModels.imageQualityOptional")}</span>
        <Input
          aria-label={t("myModels.imageQuality")}
          list="image-qualities"
          placeholder={t("myModels.imageQualityPlaceholder")}
          disabled={disabled}
          value={typeof doc.quality === "string" ? doc.quality : ""}
          onChange={(event) => onChange("quality", event.target.value || undefined)}
        />
        <datalist id="image-qualities">
          {["auto", "low", "medium", "high", "standard", "hd"].map((quality) => (
            <option key={quality} value={quality} />
          ))}
        </datalist>
      </label>
      <p className="text-xs leading-relaxed text-muted-foreground">
        {t("myModels.imageDefaultsHint")}
      </p>
    </div>
  );
}
