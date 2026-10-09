package models

import (
	"bytes"
	"github.com/sunqirui1987/xhub/internal/config"
	"net/http/httptest"
	"testing"
)

// TestRoutingGroupsStrictBody 验证非法正文、未知字段、多对象、缺失记录及503；内存宿主，目录不变，无外部清理。
func TestRoutingGroupsStrictBody(t *testing.T) {
	h := &modelTestHost{models: []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}}}
	for _, tc := range []struct {
		method, body string
		status       int
	}{{"POST", "{}", 400}, {"POST", "null", 400}, {"POST", `{"group_name":"g","models":["a"],"routing_strategy":"random","ttl":1}`, 400}, {"POST", `{"group_name":"g","models":["a"],"routing_strategy":"random"} {}`, 400}, {"POST", `{"group_name":"a","models":["a"],"routing_strategy":"random"}`, 400}, {"POST", `{"group_name":"g","models":["absent"],"routing_strategy":"random"}`, 400}, {"POST", `{"group_name":"g","models":["a"],"routing_strategy":"random"}`, 503}, {"PUT", `{"group_name":"g","models":["a"],"routing_strategy":"random"}`, 404}, {"DELETE", `{"group_name":"g"}`, 404}, {"DELETE", `{"group_name":"g","models":[]}`, 400}, {"GET", "", 200}} {
		w := httptest.NewRecorder()
		RoutingGroups(h, w, httptest.NewRequest(tc.method, "/routing/groups", bytes.NewBufferString(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s %s 状态%d: %s", tc.method, tc.body, w.Code, w.Body.String())
		}
		if len(h.models) != 2 {
			t.Fatal("非法请求修改目录")
		}
	}
}
