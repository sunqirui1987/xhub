package keys

import (
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestTeamlessKeyInput 验证创建解析；前置缺省、null、空团队以及非法归属和期限。
// 参数 t 为测试上下文，返回无；确认个人归属默认本人，服务和项目仍需团队；纯函数无清理。
func TestTeamlessKeyInput(t *testing.T) {
	for _, body := range []map[string]any{{}, {"team_id": nil}, {"team_id": ""}, {"team_id": "team"}} {
		in, err := keyInputFrom(body, "me")
		if err != nil || in.UserID != "me" || in.OwnerType != iam.OwnerPersonal {
			t.Fatalf("个人密钥不应要求团队: input=%v result=%+v err=%v", body, in, err)
		}
	}
	for _, tc := range []struct {
		body map[string]any
		user string
	}{
		{map[string]any{}, ""},
		{map[string]any{"owner_type": "service"}, "me"},
		{map[string]any{"project_id": "project"}, "me"},
		{map[string]any{"owner_type": "unknown"}, "me"},
		{map[string]any{"duration": "invalid"}, "me"},
	} {
		if _, err := keyInputFrom(tc.body, tc.user); err == nil {
			t.Fatalf("非法创建输入未拒绝: %v", tc.body)
		}
	}
}

// TestKeyModelSelection 验证模型名单归一化与更新保留语义；前置具体模型、空值、全选和异常类型。
// 参数 t 为测试上下文，返回无；全选继承权限，缺字段保留名单，具体名单不扩大；无持久副作用。
func TestKeyModelSelection(t *testing.T) {
	for _, tc := range []struct {
		raw  any
		want []string
	}{
		{nil, nil}, {42, nil}, {[]any{}, []string{}},
		{[]any{"chat"}, []string{"chat"}},
		{[]any{"all-proxy-models"}, []string{}},
		{[]any{"all-team-models"}, []string{}},
	} {
		if got := keyModels(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("模型选择 %v: got=%v want=%v", tc.raw, got, tc.want)
		}
		in, err := keyInputFrom(map[string]any{"models": tc.raw}, "me")
		if err != nil || !reflect.DeepEqual(in.Models, tc.want) {
			t.Fatalf("创建模型解析错误: %+v %v", in, err)
		}
		if got := patchFrom(&iam.Key{Models: []string{"old"}}, map[string]any{"models": tc.raw}).Models; !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("更新模型解析错误: %v", got)
		}
	}
	if got := patchFrom(&iam.Key{Models: []string{"old"}}, map[string]any{}).Models; !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatalf("缺字段覆盖原模型: %v", got)
	}
}
