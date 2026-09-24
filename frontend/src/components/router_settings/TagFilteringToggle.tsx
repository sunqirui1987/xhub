import React, { useId } from "react";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";

interface TagFilteringToggleProps {
  enabled: boolean;
  routerFieldsMetadata: { [key: string]: any };
  onToggle: (enabled: boolean) => void;
}

const TagFilteringToggle: React.FC<TagFilteringToggleProps> = ({ enabled, routerFieldsMetadata, onToggle }) => {
  const toggleId = useId();
  const tagFilteringLabel = routerFieldsMetadata["enable_tag_filtering"]?.ui_field_name || "Enable Tag Filtering";

  return (
    <div className="space-y-3 max-w-3xl">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <label htmlFor={toggleId} className="text-xs font-medium text-foreground uppercase tracking-wide">
            {t(tagFilteringLabel)}
          </label>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t(routerFieldsMetadata["enable_tag_filtering"]?.field_description || "")}
            {routerFieldsMetadata["enable_tag_filtering"]?.link && (
              <>
                {" "}
                <a
                  href={routerFieldsMetadata["enable_tag_filtering"].link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-info hover:text-info/80 underline"
                >
                  {t("Learn more")}
                </a>
              </>
            )}
          </p>
        </div>
        <Switch id={toggleId} checked={enabled} onCheckedChange={onToggle} className="ml-4" />
      </div>
    </div>
  );
};

export default TagFilteringToggle;
