import { describe, expect, it } from "vitest";
import { requestLogStatus } from "./taskStatus";

describe("task lifecycle status", () => {
  /** 前置纯展示函数；验证正常生命周期、旧日志与失败优先级，无 DOM 或数据需要清理。 */
  it("distinguishes lifecycle states and retains legacy/error behavior", () => {
    for (const [status, label] of Object.entries({ executing: "Executing", polling: "Polling", completed: "Completed", failed: "Failure", success: "Success", unknown: "Success" })) {
      expect(requestLogStatus({ status }).label).toBe(label);
    }
    expect(requestLogStatus({}).label).toBe("Success");
    expect(requestLogStatus({ status: "completed", error: "bad" }).tone).toBe("error");
  });
});
