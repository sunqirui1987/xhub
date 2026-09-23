import React from "react";
import { Minus, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { t } from "@/i18n";

export type KeyValuePair = readonly [string, string];

interface KeyValueInputProps {
  value?: readonly KeyValuePair[];
  onChange?: (value: readonly KeyValuePair[]) => void;
}

const KeyValueInput: React.FC<KeyValueInputProps> = ({ value = [], onChange }) => {
  const handleAdd = () => onChange?.([...value, ["", ""]]);

  const handleRemove = (index: number) => onChange?.(value.filter((_, i) => i !== index));

  const handleChange = (index: number, pair: KeyValuePair) =>
    onChange?.(value.map((existing, i) => (i === index ? pair : existing)));

  return (
    <div className="space-y-2">
      {value.map(([key, val], index) => (
        <div key={index} className="flex items-center gap-2">
          <Input placeholder={t("Header Name")} value={key} onChange={(e) => handleChange(index, [e.target.value, val])} />
          <Input placeholder={t("Header Value")} value={val} onChange={(e) => handleChange(index, [key, e.target.value])} />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => handleRemove(index)}
            aria-label={t("Remove header {value0}", { value0: (index + 1) })}
          >
            <Minus />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" onClick={handleAdd}>
        <Plus />
        {t("Add Header")}
      </Button>
    </div>
  );
};

export default KeyValueInput;
