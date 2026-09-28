import React from "react";
import { Download, FileSpreadsheet, FileText } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { MultiModelResult } from "./types";
import { exportMultiToPDF, exportMultiToCSV } from "./multi_export_utils";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";

interface MultiExportDropdownProps {
  multiResult: MultiModelResult;
}

const MultiExportDropdown: React.FC<MultiExportDropdownProps> = ({ multiResult }) => {
  const hasResults = multiResult.entries.some((e) => e.result !== null);

  if (!hasResults) {
    return null;
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={buttonVariants({ variant: "secondary", size: "xs" })}>
        <Download />
        {t("Export")}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuItem onClick={() => exportMultiToPDF(multiResult)}>
          <FileText />
          {t("Export as PDF")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            try {
              exportMultiToCSV(multiResult);
              toast.success(t("Exported CSV"));
            } catch (error) {
              toast.fromError(t("Failed to export CSV: ") + (error instanceof Error ? error.message : String(error)));
            }
          }}
        >
          <FileSpreadsheet />
          {t("Export as CSV")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default MultiExportDropdown;
