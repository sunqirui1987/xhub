package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOfficialTaskStatus 验证创建、轮询及失败终态转换；前置纯函数，核对缺失和未知状态，无数据需清理。
func TestOfficialTaskStatus(t *testing.T) {
	for _, tc := range []struct {
		state   string
		http    int
		initial bool
		want    string
	}{
		{"", 200, true, "executing"}, {"running", 200, false, "polling"},
		{"IN_QUEUE", 200, false, "polling"}, {"unknown", 200, false, "polling"},
		{" COMPLETED ", 200, false, "completed"}, {"succeeded", 200, false, "completed"},
		{"failed", 200, false, "failed"}, {"expired", 200, false, "failed"},
		{"running", 502, false, "failed"},
	} {
		if got := officialTaskStatus(tc.http, map[string]any{"status": tc.state}, tc.initial); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
	if officialTaskStatus(200, nil, false) != "polling" || officialTaskStatus(200, map[string]any{"error": "bad"}, true) != "failed" {
		t.Fatal("missing/error response status")
	}
}

// TestTaskFollowKeepsOriginalLifecycle 验证创建保存原请求身份，轮询、过期和网络失败沿用同一身份且失败不结算。
// 参数 t：测试上下文；前置本地 HTTP 上游和 Ark 部署；返回无，未知态仍轮询，关闭上游模拟断连并清理资源。
func TestTaskFollowKeepsOriginalLifecycle(t *testing.T) {
	response := `{"id":"task-lifecycle"}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, response)
	}))
	defer up.Close()
	dep := deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil)
	h := officialHost(up, dep)
	h.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"video"}`)
	created := h.notes[len(h.notes)-1]
	if !created.TaskInitial || created.TaskRequestID == "" || created.TaskStatus != "executing" {
		t.Fatalf("创建未保存原任务日志身份: %+v", created)
	}
	for _, tc := range []struct{ body, want string }{
		{`{"status":"running"}`, "polling"},
		{`{"status":"unknown"}`, "polling"},
		{`{"status":"expired","usage":{"completion_tokens":12}}`, "failed"},
		{`{"status":"succeeded"}`, "completed"},
	} {
		response = tc.body
		h.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/task-lifecycle", "")
		note := h.notes[len(h.notes)-1]
		if note.TaskRequestID != created.TaskRequestID || note.TaskInitial || note.TaskStatus != tc.want || note.TaskSettlement {
			t.Fatalf("查询身份、状态或费用错误: %s %+v", tc.body, note)
		}
	}
	up.Close()
	r := h.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/task-lifecycle", "")
	note := h.notes[len(h.notes)-1]
	if r.Code != http.StatusBadGateway || note.TaskRequestID != created.TaskRequestID || note.TaskStatus != "failed" || note.TaskSettlement {
		t.Fatalf("断连未更新原日志失败状态: HTTP %d %+v", r.Code, note)
	}
}
