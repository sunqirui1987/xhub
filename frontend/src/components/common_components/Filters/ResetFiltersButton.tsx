import { Button } from "@/components/ui/button";
import { RotateCcw } from "lucide-react";
import React from "react";
import { t } from "@/i18n";

interface ResetFiltersButtonProps {
  onClick: () => void;
  label?: string;
}

export const ResetFiltersButton: React.FC<ResetFiltersButtonProps> = ({ onClick, label = t("Reset Filters") }) => {
  return (
    <Button variant="outline" onClick={onClick}>
      <RotateCcw className="size-4" />
      {label}
    </Button>
  );
};
