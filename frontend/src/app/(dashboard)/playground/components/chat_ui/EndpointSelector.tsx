import { SearchSelect } from "@/components/shared/SearchSelect";
import React from "react";
import type { SearchSelectOption } from "@/components/shared/SearchSelect";
import { t } from "@/i18n";

interface EndpointSelectorProps {
  endpointType: string | null;
  onEndpointChange: (value: string | null) => void;
  className?: string;
  options: SearchSelectOption[];
  disabled?: boolean;
}

/** EndpointSelector 显示上层传入的所选模型端点，不维护全局默认选项。
 * 参数 endpointType/options/onEndpointChange：当前选择、真实绑定选项、选择回调；disabled：是否禁用。
 * 返回：可搜索的选择器。调用：ChatUI。测试：EndpointSelector.test.tsx。
 */
const EndpointSelector: React.FC<EndpointSelectorProps> = ({
  endpointType,
  onEndpointChange,
  className,
  options,
  disabled,
}) => {
  return (
    <div className={className}>
      <SearchSelect
        value={endpointType}
        onValueChange={onEndpointChange}
        options={options}
        disabled={disabled}
        emptyText="所选模型没有配置可用端点"
        placeholder={t("Select an endpoint")}
      />
    </div>
  );
};

export default EndpointSelector;
