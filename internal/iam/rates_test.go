package iam

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// TestValidateRates 验证独立限额 nil、零、INT 边界、负数与溢出；纯函数，无持久化清理。
func TestValidateRates(t *testing.T) {
	for _, v := range []int{0, 1, 2147483647, -1, 2147483648} {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			err := ValidateRates(&v, nil)
			want := v < 0 || int64(v) > 2147483647
			if (err != nil) != want {
				t.Fatalf("限额 %d 校验结果 %v", v, err)
			}
		})
	}
	if err := ValidateRates(nil, nil); err != nil {
		t.Fatal(err)
	}
}

// TestRateAllocated 验证整数固定/共享分配、透传、零与超配；纯内存树，无数据需清理。
func TestRateAllocated(t *testing.T) {
	one, two, three := 1, 2, 3
	for _, tokens := range []bool{false, true} {
		t.Run(fmt.Sprint(tokens), func(t *testing.T) {
			leaf := &quotaNode{RPM: &two, TPM: &two}
			shared := &quotaNode{Children: []*quotaNode{leaf}}
			root := &quotaNode{RPM: &three, TPM: &three, Children: []*quotaNode{shared, {RPM: &one, TPM: &one}}}
			if got, err := rateAllocated(root, tokens); err != nil || got != 3 {
				t.Fatalf("正好分满: %d %v", got, err)
			}
			root.Children = append(root.Children, &quotaNode{RPM: &one, TPM: &one})
			if _, err := rateAllocated(root, tokens); !errors.Is(err, ErrRateAllocation) {
				t.Fatalf("超配未拒绝: %v", err)
			}
		})
	}
}

// TestRateDatabaseAllocation 验证真实事务并发分配、超配回滚、转入校验和退出释放；testDB 创建并清理隔离 schema。
func TestRateDatabaseAllocation(t *testing.T) {
	db := testDB(t)
	ctx := t.Context()
	by := Actor{}
	limit, child := 10, 6
	org, err := db.CreateOrg(ctx, by, "rates-db", nil, RateLimits{RPMLimit: &limit, TPMLimit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *Team, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			team, e := db.CreateTeam(ctx, by, TeamInput{OrganizationID: org.ID, Name: fmt.Sprint("rate-", i), RPMLimit: &child, TPMLimit: &child})
			if e != nil {
				failures <- e
			} else {
				results <- team
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("并发超配成功=%d 失败=%d", len(results), len(failures))
	}
	for e := range failures {
		if !errors.Is(e, ErrRateAllocation) {
			t.Fatal(e)
		}
	}
	team := <-results
	u, e := db.CreateUser(ctx, by, UserInput{Email: "rate-db@local.invalid", Password: "password123", TeamID: team.ID, RPMLimit: &child, TPMLimit: &child})
	if e != nil {
		t.Fatal(e)
	}
	seven := 7
	patch := &seven
	if e = db.SetMemberRoleBudget(ctx, by, team.ID, u.ID, TeamAdmin, nil, RatePatch{RPMLimit: &patch, TPMLimit: &patch}); !errors.Is(e, ErrRateAllocation) {
		t.Fatalf("成员超配未拒绝: %v", e)
	}
	members, e := db.ListMembers(ctx, team.ID)
	if e != nil || len(members) != 1 || members[0].Role != TeamMember || members[0].RPMLimit == nil || *members[0].RPMLimit != 6 {
		t.Fatalf("成员角色与速率未回滚: %+v %v", members, e)
	}
	small := 4
	other, e := db.CreateTeam(ctx, by, TeamInput{OrganizationID: org.ID, Name: "small", RPMLimit: &small, TPMLimit: &small})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.RemoveMember(ctx, by, team.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.AddMember(ctx, by, other.ID, u.Email, TeamMember); !errors.Is(e, ErrRateAllocation) {
		t.Fatalf("加入团队超配未拒绝: %v", e)
	}
	memberships, e := db.MemberTeams(ctx, u.ID)
	if e != nil || len(memberships) != 0 {
		t.Fatalf("加入失败保留了归属: %+v %v", memberships, e)
	}
	plan, e := db.RatePlan(ctx, "user", u.ID)
	if e != nil || len(plan.Path) != 1 {
		t.Fatalf("退出未解除速率树: %+v %v", plan, e)
	}
	if _, e = db.CreateUser(ctx, by, UserInput{Email: "replacement@local.invalid", Password: "password123", TeamID: team.ID, RPMLimit: &child, TPMLimit: &child}); e != nil {
		t.Fatalf("退出未释放分配: %v", e)
	}
}

// TestBuildRatePlan 验证仅包含当前根的全部兄弟保留及完整四层路径；前置内存树，无数据库写入或清理。
// 同时覆盖合法独立个人、缺记录、悬空上级、多团队、循环与错配 ID，错误必须阻止构造无限根。
func TestBuildRatePlan(t *testing.T) {
	for _, tc := range []struct {
		name   string
		nodes  map[string]*quotaNode
		target string
		want   error
		count  int
		path   []string
	}{
		{"四层与兄弟", map[string]*quotaNode{
			"org:a": {ID: "a", Kind: "org"}, "team:a": {ID: "a", Kind: "team", Parent: "org:a"},
			"user:a": {ID: "a", Kind: "user", Parent: "team:a"}, "key:a": {ID: "a", Kind: "key", Parent: "user:a"},
			"team:b": {ID: "b", Kind: "team", Parent: "org:a"}, "user:b": {ID: "b", Kind: "user", Parent: "team:b"},
			"org:other": {ID: "other", Kind: "org"}, "user:independent": {ID: "independent", Kind: "user"},
		}, "key:a", nil, 6, []string{"key:a", "user:a", "team:a", "org:a"}},
		{"独立个人", map[string]*quotaNode{"user:a": {ID: "a", Kind: "user"}, "key:a": {ID: "a", Kind: "key", Parent: "user:a"}}, "key:a", nil, 2, []string{"key:a", "user:a"}},
		{"不存在", map[string]*quotaNode{}, "key:a", ErrNotFound, 0, nil},
		{"上级缺失", map[string]*quotaNode{"user:a": {ID: "a", Kind: "user", Parent: "team:missing"}}, "user:a", ErrInvalid, 0, nil},
		{"多团队", map[string]*quotaNode{"user:a": {ID: "a", Kind: "user", Parent: "ambiguous"}}, "user:a", ErrMultipleTeams, 0, nil},
		{"循环", map[string]*quotaNode{"user:a": {ID: "a", Kind: "user", Parent: "key:a"}, "key:a": {ID: "a", Kind: "key", Parent: "user:a"}}, "key:a", ErrInvalid, 0, nil},
		{"ID错配", map[string]*quotaNode{"user:a": {ID: "b", Kind: "user"}}, "user:a", ErrInvalid, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := buildRatePlan(tc.nodes, tc.target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("归属错误 got=%v want=%v", err, tc.want)
			}
			if tc.want != nil {
				if len(plan.Nodes) != 0 {
					t.Fatal("异常归属仍生成计数配置")
				}
				return
			}
			if len(plan.Nodes) != tc.count || len(plan.Path) != len(tc.path) {
				t.Fatalf("分量或路径错误: %+v", plan)
			}
			for i, id := range tc.path {
				if plan.Nodes[plan.Path[i]].ID != id {
					t.Fatalf("第%d层归属错误: %+v", i, plan)
				}
			}
			for _, node := range plan.Nodes {
				if node.ID == "org:other" || node.ID == "user:independent" {
					t.Fatal("读取了无关根的分钟计数")
				}
				if parent := tc.nodes[node.ID].Parent; parent != "" && (node.Parent < 0 || plan.Nodes[node.Parent].ID != parent) {
					t.Fatalf("父节点索引错误: %+v", node)
				}
			}
		})
	}
}
