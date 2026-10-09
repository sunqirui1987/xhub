"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";

type Option = { value: string; label: string };

/** 模型联想选择器供模型编辑器使用；参数为当前值、完整选项和回调，返回可访问输入框。
 * 聚焦已有选中值时展示全部模型，输入时才搜索；支持手动 ID，不截断结果且不发起外部请求。 */
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
  const [search, setSearch] = useState(value);
  const term = search.trim().toLowerCase();
  const matches = options.filter((option) => (option.value + " " + option.label).toLowerCase().includes(term));
  /** 将所选模型交给父表单并关闭菜单；参数为目录选项，返回无，供鼠标和键盘选择调用。 */
  const choose = (option: Option) => {
    onSelect(option.value);
    setOpen(false);
    setActive(-1);
  };
  /** 同步输入值并开始搜索；参数为输入事件，返回无，保留手动模型 ID 并重置键盘高亮。 */
  const change = (event: React.ChangeEvent<HTMLInputElement>) => {
    setSearch(event.target.value);
    onChange(event.target.value);
    setOpen(true);
    setActive(-1);
  };
  /** 处理菜单方向键、选择和退出；参数为键盘事件，返回无，空结果不选中模型或阻止提交。 */
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
        onFocus={() => { setSearch(options.some(option => option.value === value) ? "" : value); setOpen(true); setActive(-1); }}
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
