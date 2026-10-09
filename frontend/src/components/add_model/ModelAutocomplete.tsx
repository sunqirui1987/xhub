"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";

type Option = { value: string; label: string };

export default function ModelAutocomplete({
  id,
  value,
  options,
  onChange,
  onSelect,
  placeholder,
  required,
}: {
  id: string;
  value: string;
  options: Option[];
  onChange: (value: string) => void;
  onSelect: (value: string) => void;
  placeholder?: string;
  required?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const term = value.trim().toLowerCase();
  const matches = options.filter((option) => (option.value + " " + option.label).toLowerCase().includes(term));
  const choose = (option: Option) => {
    onSelect(option.value);
    setOpen(false);
    setActive(-1);
  };
  const change = (event: React.ChangeEvent<HTMLInputElement>) => {
    onChange(event.target.value);
    setOpen(true);
    setActive(-1);
  };
  const keyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") setOpen(false);
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      const direction = event.key === "ArrowDown" ? 1 : -1;
      setActive((current) => Math.max(0, Math.min(matches.length - 1, current + direction)));
    }
    if (event.key === "Enter" && open && matches[active]) {
      event.preventDefault();
      choose(matches[active]);
    }
  };
  return (
    <div className="relative">
      <Input
        id={id}
        role="combobox"
        autoComplete="off"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={id + "-options"}
        aria-activedescendant={open && active >= 0 ? id + "-option-" + active : undefined}
        value={value}
        required={required}
        placeholder={placeholder}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onChange={change}
        onKeyDown={keyDown}
      />
      {open && (
        <ul
          id={id + "-options"}
          role="listbox"
          aria-label="上游模型联想列表"
          className="absolute z-floating mt-1 max-h-72 w-full overflow-auto rounded-md border bg-popover p-1 shadow-md"
        >
          {matches.map((option, index) => (
            <li
              key={option.value}
              id={id + "-option-" + index}
              role="option"
              aria-selected={index === active}
              className={
                "cursor-pointer rounded px-3 py-2 text-sm hover:bg-accent " + (index === active ? "bg-accent" : "")
              }
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(option)}
            >
              <span className="block break-all font-medium">{option.value}</span>
              {option.label !== option.value && <span className="text-xs text-muted-foreground">{option.label}</span>}
            </li>
          ))}
          {!matches.length && <li className="p-3 text-xs text-muted-foreground">没有匹配项，将使用输入的模型 ID。</li>}
        </ul>
      )}
    </div>
  );
}
