import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import AdvancedTemplateSettings from "./AdvancedTemplateSettings";
import { emptyForm, type TemplateFormState } from "./templateForm";

vi.mock("@/i18n", () => ({ t: (key: string) => key }));
vi.mock("./ModelFallbackEditor", () => ({
  default: ({
    title,
    onChange,
  }: {
    title: string;
    onChange: (rows: { primary: string; targets: string }[]) => void;
  }) => (
    <div>
      <span>{title}</span>
      <button type="button" onClick={() => onChange([{ primary: "primary", targets: "backup" }])}>
        edit fallback
      </button>
    </div>
  ),
}));
vi.mock("./TemplateRoutingGroups", () => ({
  default: ({ onChange }: { onChange: (value: string) => void }) => (
    <button type="button" onClick={() => onChange('[{"group_name":"fast"}]')}>
      edit routing groups
    </button>
  ),
}));

const renderSettings = (form: TemplateFormState = emptyForm(), onChange = vi.fn()) => {
  render(
    <AdvancedTemplateSettings
      form={form}
      modelNames={["primary", "backup"]}
      availableStrategies={["simple-shuffle"]}
      onChange={onChange}
    />,
  );
  return onChange;
};

const chooseAndAdd = (module: string) => {
  fireEvent.change(screen.getByLabelText("pages.routeTemplates.advanced.addConfiguration"), {
    target: { value: module },
  });
  fireEvent.click(screen.getByRole("button", { name: "pages.routeTemplates.advanced.addConfiguration" }));
};

const StatefulSettings = ({ initialForm }: { initialForm: TemplateFormState }) => {
  const [form, setForm] = useState(initialForm);
  return (
    <AdvancedTemplateSettings
      form={form}
      modelNames={["primary", "backup"]}
      availableStrategies={["simple-shuffle"]}
      onChange={(patch) => setForm((current) => ({ ...current, ...patch }))}
    />
  );
};

describe("AdvancedTemplateSettings", () => {
  it("starts with a small empty state and adds only the selected configuration", () => {
    const onChange = renderSettings();

    expect(screen.getByText("pages.routeTemplates.advanced.emptyTitle")).toBeInTheDocument();
    expect(screen.queryByText("edit routing groups")).not.toBeInTheDocument();

    chooseAndAdd("routingGroups");

    expect(screen.getByText("edit routing groups")).toBeInTheDocument();
    expect(screen.queryByText("edit fallback")).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows saved sections and removing one clears only that section", () => {
    const form = {
      ...emptyForm(),
      routing_groups: '[{"group_name":"fast"}]',
      stream_timeout: "45",
      retry_policy: '{"RateLimitErrorRetries":2}',
      extra: '{"custom_router_flag":true}',
    };
    const onChange = renderSettings(form);

    fireEvent.click(
      screen.getByRole("button", {
        name: "pages.routeTemplates.advanced.remove pages.routeTemplates.advanced.modules.routingGroups",
      }),
    );

    expect(onChange).toHaveBeenCalledWith({ routing_groups: "" });
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("stream_timeout");
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("retry_policy");
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("extra");
  });

  it("edits an added fallback chain with a field-only patch", () => {
    const form = { ...emptyForm(), stream_timeout: "30", model_group_alias: '{"old":"new"}' };
    const onChange = renderSettings(form);

    chooseAndAdd("contextFallbacks");
    fireEvent.click(screen.getByRole("button", { name: "edit fallback" }));

    expect(onChange).toHaveBeenLastCalledWith({
      context_window_fallbacks: [{ primary: "primary", targets: "backup" }],
    });
    expect(onChange.mock.calls.at(-1)?.[0]).not.toHaveProperty("stream_timeout");
    expect(onChange.mock.calls.at(-1)?.[0]).not.toHaveProperty("model_group_alias");
  });

  it("removes one compatibility option without clearing other saved options", () => {
    const form = { ...emptyForm(), stream_timeout: "30", retry_after: "7", enable_pre_call_checks: true };
    const onChange = renderSettings(form);

    fireEvent.click(
      screen.getByRole("button", {
        name: "pages.routeTemplates.advanced.remove pages.routeTemplates.advanced.modules.retryAfter",
      }),
    );

    expect(onChange).toHaveBeenCalledWith({ retry_after: "" });
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("stream_timeout");
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("enable_pre_call_checks");
  });

  it("removes fallback limits without resetting independently configured causes", () => {
    const form = { ...emptyForm(), max_fallbacks: "9", fallback_causes: ["status"] };
    const onChange = renderSettings(form);

    fireEvent.click(
      screen.getByRole("button", {
        name: "pages.routeTemplates.advanced.remove pages.routeTemplates.advanced.modules.maxFallbacks",
      }),
    );

    expect(onChange).toHaveBeenCalledWith({ max_fallbacks: "" });
    expect(onChange.mock.calls[0][0]).not.toHaveProperty("fallback_causes");
  });

  it("hides a saved fallback limit after the parent applies its removal patch", () => {
    render(<StatefulSettings initialForm={{ ...emptyForm(), max_fallbacks: "9" }} />);
    const removeName = "pages.routeTemplates.advanced.remove pages.routeTemplates.advanced.modules.maxFallbacks";

    fireEvent.click(screen.getByRole("button", { name: removeName }));

    expect(screen.queryByRole("button", { name: removeName })).not.toBeInTheDocument();
    expect(
      screen.getByText("pages.routeTemplates.advanced.modules.maxFallbacks", { selector: "option" }),
    ).toBeInTheDocument();
  });
});
