import { t } from "@/i18n";
export type ModelViewType = "groups" | "individual";

const MODEL_VIEW_OPTIONS: readonly { value: ModelViewType; labelKey: string }[] = [
  { value: "groups", labelKey: "Public Model Name" },
  { value: "individual", labelKey: "Litellm Model Name" },
];

interface ModelViewToggleProps {
  value: ModelViewType;
  onChange: (value: ModelViewType) => void;
}

export default function ModelViewToggle({ value, onChange }: ModelViewToggleProps) {
  return (
    <div className="flex bg-muted rounded-lg p-1">
      {MODEL_VIEW_OPTIONS.map((option) => (
        <button
          key={option.value}
          className={`px-3 py-1 text-sm rounded-md transition-colors ${
            value === option.value ? "bg-card shadow-xs text-foreground" : "text-muted-foreground hover:text-foreground"
          }`}
          onClick={() => onChange(option.value)}
        >
          {t(option.labelKey)}
        </button>
      ))}
    </div>
  );
}
