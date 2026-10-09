package main

import "testing"

// TestExpectedOperationalFailure 验证 sweep 只放行未配置价格源的精确失败，其他方法、状态和 502 仍失败；无外部数据且无需清理。
func TestExpectedOperationalFailure(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		code   int
		body   string
		want   bool
	}{
		{name: "明确未配置价格源", method: "POST", path: "/reload/model_cost_map", code: 502, body: "price feed URL must be explicitly configured", want: true},
		{name: "其他服务端错误", method: "POST", path: "/reload/model_cost_map", code: 502, body: "dial failed"},
		{name: "错误方法", method: "GET", path: "/reload/model_cost_map", code: 502, body: "price feed URL must be explicitly configured"},
		{name: "错误状态", method: "POST", path: "/reload/model_cost_map", code: 500, body: "price feed URL must be explicitly configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expectedOperationalFailure(tt.method, tt.path, tt.code, tt.body); got != tt.want {
				t.Fatalf("expectedOperationalFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}
