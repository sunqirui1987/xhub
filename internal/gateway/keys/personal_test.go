package keys

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestPersonalFilter 验证 SQL 本人范围；前置各类权限与身份，确认管理员、团队管理员均不能扩大到他人或服务密钥。
// 参数 t 为测试上下文，返回无；纯函数测试，无持久数据需要清理。
func TestPersonalFilter(t *testing.T) {
	for _, base := range []iam.KeyFilter{{}, {OwnOrService: true, TeamIDs: []string{"team"}, UserID: "other"}, {KeyIDs: []string{"key"}}} {
		got := personalFilter(base, &auth.Principal{Kind: authz.KindSession, UserID: "me", Role: "admin"})
		if base.OwnOrService && got.TeamIDs != nil {
			t.Fatal("个人分支错误地继承服务密钥团队限制")
		}
		if got.UserID != "me" || got.OwnerType != iam.OwnerPersonal || got.OwnOrService || !reflect.DeepEqual(got.KeyIDs, base.KeyIDs) {
			t.Fatalf("个人范围未正确收窄: %#v", got)
		}
	}
	for _, p := range []*auth.Principal{nil, {}, {Kind: authz.KindKey, UserID: "me"}, {Kind: authz.KindSession}} {
		if got := personalFilter(iam.KeyFilter{}, p); got.KeyIDs == nil || len(got.KeyIDs) != 0 {
			t.Fatalf("缺失或非会话身份未拒绝查询: %#v", got)
		}
	}
}

// TestPersonalKeyVisible 验证个人详情归属；前置本人、他人、空记录和服务密钥，只有本人个人密钥可见。
// 参数 t 为测试上下文，返回无；内存数据无需清理。
func TestPersonalKeyVisible(t *testing.T) {
	me, other := "me", "other"
	p := &auth.Principal{Kind: authz.KindSession, UserID: me}
	for _, tc := range []struct {
		key  *iam.Key
		want bool
	}{
		{&iam.Key{OwnerType: iam.OwnerPersonal, UserID: &me}, true},
		{&iam.Key{OwnerType: iam.OwnerPersonal, UserID: &other}, false},
		{&iam.Key{OwnerType: iam.OwnerPersonal}, false},
		{&iam.Key{OwnerType: iam.OwnerService, UserID: &me}, false}, {nil, false},
	} {
		if got := personalKeyVisible(p, tc.key); got != tc.want {
			t.Fatalf("详情归属错误: key=%#v got=%v", tc.key, got)
		}
		if personalKeyVisible(nil, tc.key) {
			t.Fatal("无身份可读取详情")
		}
	}
}

// TestPersonalPage 验证授权范围内项目/别名/搜索/用户筛选、排序、分页和错误输入；前置三条内存记录。
// 参数 t 为测试上下文，返回无；验证空集不回退全量、页数正确、坏参数拒绝及切片不被改写，无清理副作用。
func TestPersonalPage(t *testing.T) {
	me, projectA, projectB := "me", "project-a", "project-b"
	rows := []iam.Key{{ID: "c", Name: "Charlie", ProjectID: &projectB, UserID: &me}, {ID: "a", Name: "Alpha", ProjectID: &projectA, UserID: &me}, {ID: "b", Name: "Beta", ProjectID: &projectA, UserID: &me}}
	for _, tc := range []struct {
		query        string
		ids          []string
		total, pages int
		fail         bool
	}{
		{"sort_by=key_alias&sort_order=asc&size=2&page=2", []string{"c"}, 3, 2, false},
		{"search=ALPHA", []string{"a"}, 1, 1, false},
		{"project_id=project-a&size=1&page=2&sort_by=key_alias&sort_order=asc", []string{"b"}, 2, 2, false},
		{"key_alias=Alpha", []string{"a"}, 1, 1, false},
		{"key_alias=alp&substring_matching=true", []string{"a"}, 1, 1, false},
		{"user_id=other", []string{}, 0, 1, false},
		{"user_id=m", []string{}, 0, 1, false},
		{"key_hash=unknown", []string{}, 0, 1, false},
		{"page=100000", []string{}, 3, 1, false},
		{"page=0", nil, 0, 0, true}, {"size=101", nil, 0, 0, true},
		{"page=oops", nil, 0, 0, true}, {"sort_by=secret", nil, 0, 0, true}, {"sort_order=bad", nil, 0, 0, true},
	} {
		q, _ := url.ParseQuery(tc.query)
		got, total, _, _, pages, err := personalPage(rows, q)
		if (err != nil) != tc.fail {
			t.Fatalf("参数 %s 错误状态不符: %v", tc.query, err)
		}
		if tc.fail {
			continue
		}
		ids := []string{}
		for _, k := range got {
			ids = append(ids, k.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) || total != tc.total || pages != tc.pages {
			t.Fatalf("参数 %s 分页错误: ids=%v total=%d pages=%d", tc.query, ids, total, pages)
		}
	}
	if rows[0].ID != "c" {
		t.Fatal("排序修改了调用方切片")
	}
	budget := 10.0
	for field, want := range map[string]float64{"spend": 2, "max_budget": 10, "budget_remaining": 8, "budget_utilization": .2} {
		if got := personalSortValue(iam.Key{Spend: 2, MaxBudget: &budget}, field); got != want {
			t.Fatalf("%s 排序值=%v want=%v", field, got, want)
		}
	}
	if personalSortValue(iam.Key{}, "budget_utilization") != 0 {
		t.Fatal("无限预算排序应为零")
	}
}
