import { cx } from "@/lib/cva.config";
import React from "react";

interface BaseActionButtonProps {
  icon: React.ComponentType<React.ComponentProps<"svg">>;
  onClick: () => void;
  className?: string;
  disabled?: boolean;
  dataTestId?: string;
  ariaLabel?: string;
}

/** 渲染可键盘操作的图标按钮；参数提供图标、回调、名称和禁用态，返回原生按钮。
 * 表格操作使用此组件；禁用时阻止事件，type=button 避免触发表单提交。
 */
export default function BaseActionButton({
  icon: Icon,
  onClick,
  className,
  disabled,
  dataTestId,
  ariaLabel,
}: BaseActionButtonProps) {
  return (
    <button
      type="button"
      disabled={disabled}
      aria-label={ariaLabel}
      className={cx(
        "inline-flex shrink-0 items-center justify-center p-1.5",
        disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer",
        className,
      )}
      onClick={onClick}
      data-testid={dataTestId}
    >
      <Icon className="size-5 shrink-0" aria-hidden="true" />
    </button>
  );
}
