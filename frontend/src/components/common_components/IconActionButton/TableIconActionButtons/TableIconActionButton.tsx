import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import {
  PencilAltIcon,
  PlayIcon,
  RefreshIcon,
  TrashIcon,
  ChevronUpIcon,
  ChevronDownIcon,
  ExternalLinkIcon,
  ClipboardCopyIcon,
} from "@heroicons/react/outline";
import BaseActionButton from "../BaseActionButton";

export interface TableIconActionButtonProps {
  onClick: () => void;
  tooltipText?: string;
  disabled?: boolean;
  disabledTooltipText?: string;
  dataTestId?: string;
  variant: keyof typeof TableIconActionButtonMap;
}

export interface TableIconActionButtonBaseProps {
  icon: React.ComponentType<React.ComponentProps<"svg">>;
  className?: string;
}

export const TableIconActionButtonMap: Record<string, TableIconActionButtonBaseProps> = {
  Edit: { icon: PencilAltIcon, className: "hover:text-info" },
  Delete: { icon: TrashIcon, className: "hover:text-destructive" },
  Test: { icon: PlayIcon, className: "hover:text-info" },
  Regenerate: { icon: RefreshIcon, className: "hover:text-success" },
  Up: { icon: ChevronUpIcon, className: "hover:text-info" },
  Down: { icon: ChevronDownIcon, className: "hover:text-info" },
  Open: { icon: ExternalLinkIcon, className: "hover:text-success" },
  Copy: { icon: ClipboardCopyIcon, className: "hover:text-info" },
};

/** 组合表格图标按钮及提示；参数提供动作、已翻译说明与禁用原因，返回带可访问名称的按钮。
 * 调用方为管理表格；禁用原因只改变提示，保留动作名称便于辅助技术识别。
 */
export default function TableIconActionButton({
  onClick,
  tooltipText,
  disabled = false,
  disabledTooltipText,
  dataTestId,
  variant,
}: TableIconActionButtonProps) {
  const { icon, className } = TableIconActionButtonMap[variant];
  const title = disabled ? disabledTooltipText : tooltipText;
  const button = (
    <BaseActionButton
      icon={icon}
      onClick={onClick}
      className={className}
      disabled={disabled}
      dataTestId={dataTestId}
      ariaLabel={tooltipText || variant}
    />
  );

  if (!title) {
    return <span>{button}</span>;
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<span />}>{button}</TooltipTrigger>
        <TooltipContent>{title}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
