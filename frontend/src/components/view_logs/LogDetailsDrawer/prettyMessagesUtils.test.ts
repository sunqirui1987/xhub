import { describe, expect, it } from "vitest";
import { loggedResponse, parseMessages, requestBody, requestHeaders } from "./prettyMessagesUtils";

const completed = {
  type: "response.completed",
  response: {
    output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "Could you clarify?" }] }],
  },
};

describe("request panels", () => {
  it("splits a proxy document into headers and the message body", () => {
    const stored = {
      method: "POST",
      headers: { Authorization: "***", "Content-Type": "application/json" },
      body: { messages: [{ role: "user", content: "hello" }] },
    };
    expect(requestHeaders(stored)).toEqual({ Authorization: "***", "Content-Type": "application/json" });
    expect(parseMessages(stored, null).requestMessages[0]?.content).toBe("hello");
    expect(requestBody(stored)).toEqual(stored.body);
  });
});

describe("loggedResponse", () => {
  it("reads the completed response out of a stored event stream", () => {
    const stored = {
      body: `event: response.created\ndata: {"type":"response.created"}\n\nevent: response.completed\ndata: ${JSON.stringify(completed)}\n`,
    };
    const message = parseMessages(null, stored).responseMessage;
    expect(message?.content).toBe("Could you clarify?");
    expect(loggedResponse(stored)).toEqual(completed.response);
  });

  it("keeps a streamed tool call as the assistant output", () => {
    const stored = {
      body: [
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"exec","arguments":"{\\"input\\":"}}]}}]}',
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\"ls\\""}}]}}]}',
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}',
        "data: [DONE]",
      ].join("\n"),
    };
    const message = parseMessages(null, stored).responseMessage;
    expect(message?.toolCalls?.[0]?.name).toBe("exec");
    expect(message?.toolCalls?.[0]?.arguments).toEqual({ input: "ls" });
  });

  it("joins chat stream deltas into one assistant message", () => {
    const stored = {
      body: [
        'data: {"choices":[{"delta":{"content":"Hel"}}]}',
        'data: {"choices":[{"delta":{"content":"lo"}}]}',
        "data: [DONE]",
      ].join("\n"),
    };
    expect(parseMessages(null, stored).responseMessage?.content).toBe("Hello");
  });
});
