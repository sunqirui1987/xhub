import React from "react";
import type { DateRangePickerValue } from "@/components/shared/date_picker_types";
import { t } from "@/i18n";

interface ExportSummaryProps {
  dateRange: DateRangePickerValue;
  selectedFilters: string[];
}

const ExportSummary: React.FC<ExportSummaryProps> = ({ dateRange, selectedFilters }) => {
  return (
    <div className="text-sm text-muted-foreground">
      {dateRange.from?.toLocaleDateString()} - {dateRange.to?.toLocaleDateString()}
      {selectedFilters.length > 0 && t("· {value0} filter{value1}", { value0: (selectedFilters.length), value1: (selectedFilters.length > 1 ? "s" : "") })}
    </div>
  );
};

export default ExportSummary;
