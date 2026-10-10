package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/live"
)

// TestPersistSpendKeepsFailureResponseWithoutPrompts 验证直接落库保留完整失败正文。
// 参数 t 为测试上下文；前置关闭提示词保存，覆盖长 JSON、纯文本、空响应和失败任务；
// 结果应保留失败响应且不保存成功响应或请求，数据库由隔离 schema 自动清理。
func TestPersistSpendKeepsFailureResponseWithoutPrompts(t *testing.T) {
	db := testIdentityStore(t)
	s := &Server{IAM: db}
	start := time.Now().UTC()
	for _, tc := range []struct {
		name, status, response, want string
		httpStatus                   int
	}{
		{"json", "error", `{"error":{"message":"` + strings.Repeat("诊断", 2048) + `","tail":"last-cause"}}`, `{"error":{"message":"` + strings.Repeat("诊断", 2048) + `","tail":"last-cause"}}`, 502},
		{"text", "error", "upstream failed\nterminal cause", "upstream failed\nterminal cause", 400},
		{"task", "failed", `{"status":"failed","error":"task failure"}`, `{"status":"failed","error":"task failure"}`, 200},
		{"empty", "error", "", "", 502},
		{"success", "success", "private successful response", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := live.SpendLog{RequestID: "error-body-" + tc.name, Model: "test", Status: tc.status, HTTPStatus: tc.httpStatus, Response: tc.response}
			if tc.status != "success" {
				row.Error = tc.response
			}
			s.persistSpend(row, 0, promptExchange{}, start, start)
			body, err := db.GetRequestLog(context.Background(), row.RequestID)
			if err != nil {
				t.Fatalf("读取落库错误正文失败: %v", err)
			}
			if body.ResponseBody != tc.want || body.Error != row.Error || body.RequestBody != "" || body.ProxyRequest != "" {
				t.Fatalf("关闭提示词保存后错误日志不完整或泄露请求: response length=%d want=%d error length=%d", len(body.ResponseBody), len(tc.want), len(body.Error))
			}
		})
	}
}
