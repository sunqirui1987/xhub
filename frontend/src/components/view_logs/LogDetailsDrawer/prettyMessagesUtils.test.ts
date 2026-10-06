import { describe, expect, it } from "vitest";
import { loggedResponse, parseMessages } from "./prettyMessagesUtils";

const completed = {
  type: "response.completed",
  response: {
    output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "Could you clarify?" }] }],
  },
};

describe("loggedResponse", () => {
  it("reads the completed response out of a stored event stream", () => {
    const stored = {
      body: `event: response.created\ndata: {"type":"response.created"}\n\nevent: response.completed\ndata: ${JSON.stringify(completed)}\n`,
    };
    const message = parseMessages(null, stored).responseMessage;
    expect(message?.content).toBe("Could you clarify?");
    expect(loggedResponse(stored)).toEqual(completed.response);
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
