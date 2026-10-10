import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { PrettyMessagesView } from "./PrettyMessagesView";

describe("PrettyMessagesView", () => {
  it("should render the component for standard chat completions", () => {
    const request = {
      messages: [{ role: "user", content: "Hello" }],
    };
    const response = {
      choices: [{ message: { role: "assistant", content: "Hi there!" } }],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Hello")).toBeInTheDocument();
    expect(screen.getByText("Hi there!")).toBeInTheDocument();
  });

  it("renders input when request is a bare messages array (cold storage payload)", () => {
    const request = [{ role: "user", content: "Write me a poem" }];
    const response = {
      choices: [{ message: { role: "assistant", content: "A quiet moment." } }],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Write me a poem")).toBeInTheDocument();
    expect(screen.getByText("A quiet moment.")).toBeInTheDocument();
  });

  it("should render the realtime pretty view for realtime API responses", () => {
    const request = {};
    const response = {
      results: [
        {
          type: "session.created",
          session: {
            id: "sess_123",
            model: "gpt-4o-mini-realtime-preview",
            voice: "alloy",
            modalities: ["audio", "text"],
          },
        },
        {
          type: "response.done",
          response: {
            id: "resp_1",
            status: "completed",
            output: [
              {
                id: "item_1",
                role: "assistant",
                type: "message",
                content: [{ type: "audio", transcript: "Hello from realtime!" }],
              },
            ],
          },
        },
      ],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Session")).toBeInTheDocument();
    expect(screen.getByText("Hello from realtime!")).toBeInTheDocument();
    const modelElements = screen.getAllByText("gpt-4o-mini-realtime-preview");
    expect(modelElements.length).toBeGreaterThanOrEqual(1);
  });

  /** 目的：真实 Responses 外层 id/status 不触发视频视图；前置完整文本回执，验证输入与回答可见且无媒体卡片，组件由测试框架卸载。 */
  it("renders a Responses API log, whose body uses input/output instead of messages/choices", () => {
    const request = {
      model: "gpt-5.6",
      input: [{ role: "user", content: "Reply with exactly: hello from responses api" }],
    };
    const response = {
      id: "resp-text",
      object: "response",
      status: "completed",
      output: [
        {
          id: "msg_070989277645d4ae",
          role: "assistant",
          type: "message",
          status: "completed",
          content: [{ text: "hello from responses api", type: "output_text", annotations: [] }],
        },
      ],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Reply with exactly: hello from responses api")).toBeInTheDocument();
    expect(screen.getByText("hello from responses api")).toBeInTheDocument();
    expect(screen.queryByTestId("media-request")).not.toBeInTheDocument();
    expect(screen.queryByText("No response data available")).not.toBeInTheDocument();
  });

  it("renders a Responses API tool call, whose output item is a function_call", () => {
    const request = {
      model: "gpt-5.6",
      input: [{ role: "user", content: "What is the weather in San Francisco? Use the tool." }],
    };
    const response = {
      output: [
        {
          id: "fc_08edf6c2312f1485",
          name: "get_weather",
          type: "function_call",
          status: "completed",
          call_id: "call_AtO0J9eNy5jgECXzBicMJM8W",
          arguments: '{"city":"San Francisco"}',
        },
      ],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("What is the weather in San Francisco? Use the tool.")).toBeInTheDocument();
    expect(screen.getByText("get_weather")).toBeInTheDocument();
    expect(screen.queryByText("No response data available")).not.toBeInTheDocument();
  });

  it("renders instructions as the system turn and a bare string input", () => {
    const request = { model: "gpt-5.6", instructions: "You are terse.", input: "Say A" };
    const response = {
      output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "A" }] }],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("You are terse.")).toBeInTheDocument();
    expect(screen.getByText("Say A")).toBeInTheDocument();
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("skips reasoning output items rather than rendering them as empty turns", () => {
    const request = { input: [{ role: "user", content: "Think then answer" }] };
    const response = {
      output: [
        { type: "reasoning", id: "rs_1", summary: [] },
        { type: "message", role: "assistant", content: [{ type: "output_text", text: "answered" }] },
      ],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("answered")).toBeInTheDocument();
    expect(screen.queryByText("No response data available")).not.toBeInTheDocument();
  });

  it("renders a Responses API follow-up turn carrying a prior function_call and its output", () => {
    const request = {
      input: [
        { role: "user", content: "What is the weather in San Francisco? Use the tool." },
        {
          type: "function_call",
          name: "get_weather",
          call_id: "call_AtO0J9eNy5jgECXzBicMJM8W",
          arguments: '{"city":"San Francisco"}',
        },
        { type: "function_call_output", call_id: "call_AtO0J9eNy5jgECXzBicMJM8W", output: '{"temp":18}' },
      ],
    };
    const response = {
      output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "It is 18 degrees." }] }],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("It is 18 degrees.")).toBeInTheDocument();
    expect(screen.getByText('{"temp":18}')).toBeInTheDocument();
    expect(screen.getByText("TOOL")).toBeInTheDocument();
  });

  it("maps the developer and legacy function roles onto the roles the drawer renders", () => {
    const request = {
      messages: [
        { role: "developer", content: "Stay terse." },
        { role: "user", content: "Weather?" },
        { role: "function", name: "get_weather", content: '{"temp":18}' },
      ],
    };
    const response = { choices: [{ message: { role: "assistant", content: "18 degrees." } }] };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Stay terse.")).toBeInTheDocument();
    expect(screen.getByText("TOOL")).toBeInTheDocument();
    expect(screen.queryByText("FUNCTION")).not.toBeInTheDocument();
  });

  it("still reports missing output when a Responses API log has an empty output array", () => {
    const request = { input: [{ role: "user", content: "Hello" }] };

    render(<PrettyMessagesView request={request} response={{ output: [] }} />);
    expect(screen.getByText("Hello")).toBeInTheDocument();
    expect(screen.getByText("No response data available")).toBeInTheDocument();
  });

  it("should render standard view when response has results but no realtime events", () => {
    const request = {
      messages: [{ role: "user", content: "Test" }],
    };
    const response = {
      results: [{ type: "some.other.type" }],
      choices: [{ message: { role: "assistant", content: "Reply" } }],
    };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("Test")).toBeInTheDocument();
    expect(screen.getByText("Reply")).toBeInTheDocument();
  });

  it("renders an OpenAI image bypass request and a base64 image response", () => {
    const request = {
      method: "POST",
      url: "/bypass/openai/v1/images/generations",
      headers: { Authorization: "***" },
      body: { model: "gpt-image-2", prompt: "A red apple on white.", size: "1024x1024" },
    };
    const response = { data: [{ b64_json: "/9j/2Q==" }] };

    render(<PrettyMessagesView request={request} response={response} />);
    expect(screen.getByText("gpt-image-2")).toBeInTheDocument();
    expect(screen.getByText("A red apple on white.")).toBeInTheDocument();
    expect(screen.getByTestId("media-response-image")).toHaveAttribute("src", "data:image/jpeg;base64,/9j/2Q==");
    expect(screen.queryByText("No response data available")).not.toBeInTheDocument();
  });

  it("renders an image URL returned by an image generation provider", () => {
    render(
      <PrettyMessagesView
        request={{ method: "POST", url: "/v1/images/generations", body: { prompt: "A lighthouse" } }}
        response={{ data: [{ url: "https://example.invalid/lighthouse.png" }] }}
      />,
    );
    expect(screen.getByTestId("media-response-image")).toHaveAttribute("src", "https://example.invalid/lighthouse.png");
  });

  it("renders a FAL video task creation response with its local URLs", () => {
    render(
      <PrettyMessagesView
        request={{
          method: "POST",
          url: "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video",
          body: { prompt: "A static apple", duration: "5" },
        }}
        response={{
          request_id: "qvideo-1",
          status: "IN_QUEUE",
          response_url: "/queue/fal-ai/kling-video/requests/qvideo-1",
          status_url: "/queue/fal-ai/kling-video/requests/qvideo-1/status",
        }}
      />,
    );
    expect(screen.getByText("qvideo-1")).toBeInTheDocument();
    expect(screen.getByText("IN_QUEUE")).toBeInTheDocument();
    expect(screen.getByText("A static apple")).toBeInTheDocument();
    expect(screen.getByText("/queue/fal-ai/kling-video/requests/qvideo-1/status")).toBeInTheDocument();
  });

  it("renders Ark and FAL terminal video responses as playable videos", () => {
    const { rerender } = render(
      <PrettyMessagesView
        request={{ method: "GET", url: "/v3/contents/generations/tasks/qvideo-ark" }}
        response={{ id: "qvideo-ark", status: "succeeded", content: { video_url: "https://example.invalid/ark.mp4" }, duration: 4, usage: { completion_tokens: 12 } }}
      />,
    );
    expect(screen.getByTestId("media-response-video")).toHaveAttribute("src", "https://example.invalid/ark.mp4");
    expect(screen.getByTestId("media-response-usage")).toHaveTextContent("completion_tokens");

    rerender(
      <PrettyMessagesView
        request={{ method: "GET", url: "/queue/bytedance/seedance-2.0/requests/qvideo-fal" }}
        response={{ video: { url: "https://example.invalid/fal.mp4", duration: 5 } }}
      />,
    );
    expect(screen.getByTestId("media-response-video")).toHaveAttribute("src", "https://example.invalid/fal.mp4");
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("uses the persisted model id and nested FAL usage when the bypass body omits model", () => {
    render(
      <PrettyMessagesView
        model="fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
        request={{ method: "GET", url: "/queue/fal-ai/kling-video/requests/qvideo-1/status", body: null }}
        response={{
          request_id: "qvideo-1",
          status: "COMPLETED",
          result: {
            usage: { completion_tokens: 40594 },
            video: { url: "https://example.invalid/kling.mp4", duration: 5 },
          },
        }}
      />,
    );

    expect(screen.getByTestId("media-request")).toHaveTextContent(
      "fal-ai/kling-video/v2.5-turbo/pro/text-to-video",
    );
    expect(screen.getByTestId("media-response-usage")).toHaveTextContent("40594");
  });
});
