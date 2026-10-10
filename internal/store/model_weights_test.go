package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// openModelWeightStores 为权重事务测试打开同一私有 PostgreSQL schema 中的配置库和身份库。
// 参数 t 是当前测试；返回配置存储与身份存储，供测试写入真实配置、部署和模板表。
// 调用场景：本文件全部测试。数据库不可达时遵循严格模式；结束时关闭连接并删除整个 schema。
func openModelWeightStores(t *testing.T) (*Store, *iam.DB) {
	t.Helper()
	dsn := testsupport.Postgres(t, "model_weights")
	db, err := iam.Open(t.Context(), dsn)
	if err != nil {
		t.Fatalf("打开身份库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st, err := Open(dsn)
	if err != nil {
		t.Fatalf("打开配置库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Engine.Close() })
	return st, db
}

// weightDeployment 构造权重清理所需的最小部署。
// 参数 name 是公开模型名，id 是稳定部署 ID；返回完整目录可用的条目。
// 调用场景：本文件全部测试。函数不访问外部资源且没有副作用。
func weightDeployment(name, id string) config.ModelEntry {
	return config.ModelEntry{ModelName: name, ModelInfo: map[string]any{"id": id}}
}

// allocationIDs 从动态默认权重中读取部署 ID，供测试比较清理前后的引用。
// 参数 t 是当前测试，value 是 ListConfig 返回值；返回持久化顺序中的 ID。
// 调用场景：默认权重清理测试。结构损坏会终止测试；函数没有数据库副作用。
func allocationIDs(t *testing.T, value any) []string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Allocations []struct {
			ID string `json:"deployment_id"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("默认权重结构损坏: %v", err)
	}
	out := make([]string, 0, len(doc.Allocations))
	for _, row := range doc.Allocations {
		out = append(out, row.ID)
	}
	return out
}

// TestSaveModelDirectoryRollsBackOnBrokenTemplate 验证损坏模板会回滚部署写入和此前执行的默认权重清理。
// 前置条件：独立 PostgreSQL schema 中保存完整默认权重和故意损坏的客户模板。
// 验证结果：事务返回错误，新部署不可见，默认权重不变；测试结束删除整个 schema。
func TestSaveModelDirectoryRollsBackOnBrokenTemplate(t *testing.T) {
	st, db := openModelWeightStores(t)
	original := map[string]any{"allocations": []any{
		map[string]any{"deployment_id": "d1", "weight": 1},
		map[string]any{"deployment_id": "d2", "weight": 1},
	}}
	if err := st.PutConfig("model_defaults", "shared", original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteTemplate(t.Context(), iam.Actor{Kind: "system"}, iam.TemplateOwner{}, "broken", "{\"model_routes\":"); err != nil {
		t.Fatal(err)
	}
	entry := ProxyModel{ID: "new", ModelName: "shared", Params: map[string]any{}, Info: map[string]any{"id": "new"}}
	if err := st.SaveModelDirectory(&entry, "", []config.ModelEntry{weightDeployment("shared", "d1"), weightDeployment("shared", "new")}); err == nil {
		t.Fatal("损坏模板未使事务失败")
	}
	models, err := st.ListProxyModels()
	if err != nil || len(models) != 0 {
		t.Fatalf("失败事务部署泄漏: models=%v err=%v", models, err)
	}
	defaults, err := st.ListConfig("model_defaults")
	if err != nil {
		t.Fatal(err)
	}
	ids := allocationIDs(t, defaults["shared"])
	if len(ids) != 2 || ids[0] != "d1" || ids[1] != "d2" {
		t.Fatalf("失败事务修改了原默认权重: %v", defaults["shared"])
	}
}

// TestSaveModelDirectoryCleansDefaultAndTemplateReferences 验证删除、改名、零权重及新增部署时的持久化清理。
// 前置条件：各子测试在独立 schema 保存同名部署的默认权重和客户模板分配。
// 验证结果：单部署、改名及剩余全零恢复默认；新增部署不破坏已有零权重并默认继承；schema 自动清理。
func TestSaveModelDirectoryCleansDefaultAndTemplateReferences(t *testing.T) {
	for _, tc := range []struct {
		name      string
		directory []config.ModelEntry
		weights   []any
		wantIDs   []string
	}{
		{"创建部署保留已有权重", []config.ModelEntry{weightDeployment("shared", "d1"), weightDeployment("shared", "d2"), weightDeployment("shared", "d3")}, []any{map[string]any{"deployment_id": "d1", "weight": 0}, map[string]any{"deployment_id": "d2", "weight": 1}}, []string{"d1", "d2"}},
		{"删除到单部署恢复默认", []config.ModelEntry{weightDeployment("shared", "d1")}, []any{map[string]any{"deployment_id": "d1", "weight": 0}, map[string]any{"deployment_id": "d2", "weight": 1}}, nil},
		{"删除正权重后全零恢复默认", []config.ModelEntry{weightDeployment("shared", "d1"), weightDeployment("shared", "d2")}, []any{map[string]any{"deployment_id": "d1", "weight": 0}, map[string]any{"deployment_id": "d2", "weight": 0}, map[string]any{"deployment_id": "gone", "weight": 1}}, nil},
		{"改名清除旧模型引用", []config.ModelEntry{weightDeployment("renamed", "d1"), weightDeployment("shared", "d2")}, []any{map[string]any{"deployment_id": "d1", "weight": 1}, map[string]any{"deployment_id": "d2", "weight": 1}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, db := openModelWeightStores(t)
			if err := st.PutConfig("model_defaults", "shared", map[string]any{"allocations": tc.weights}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(map[string]any{"model_routes": []any{map[string]any{"model": "shared", "allocations": tc.weights}}})
			tmpl, err := db.CreateRouteTemplate(t.Context(), iam.Actor{Kind: "system"}, iam.TemplateOwner{}, "weights", string(body))
			if err != nil {
				t.Fatal(err)
			}
			if err := st.SaveModelDirectory(nil, "", tc.directory); err != nil {
				t.Fatalf("清理目录失败: %v", err)
			}
			defaults, err := st.ListConfig("model_defaults")
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.wantIDs) == 0 {
				if _, exists := defaults["shared"]; exists {
					t.Fatalf("应恢复默认却仍保存权重: %v", defaults["shared"])
				}
			} else if ids := allocationIDs(t, defaults["shared"]); len(ids) != 2 || ids[0] != "d1" || ids[1] != "d2" {
				t.Fatalf("创建后默认引用错误: %v", ids)
			}
			got, err := db.GetRouteTemplate(t.Context(), tmpl.ID)
			if err != nil {
				t.Fatal(err)
			}
			var cleaned map[string]any
			if err := json.Unmarshal([]byte(got.Body), &cleaned); err != nil {
				t.Fatal(err)
			}
			rule := cleaned["model_routes"].([]any)[0].(map[string]any)
			if len(tc.wantIDs) == 0 && rule["allocations"] != nil {
				t.Fatalf("模板仍保存失效分配: %v", rule)
			}
		})
	}
}

// TestSaveModelDirectoryLocksLatestTemplateRow 验证 PostgreSQL FOR UPDATE 等待并发模板编辑并读取其提交结果。
// 前置条件：独立 schema 中锁住模板行，先写未参与清理的 retry_policy，再并发保存模型目录。
// 验证结果：保存不会越过行锁造成 lost write；锁释放后保留新字段并清除悬空引用；schema 自动清理。
func TestSaveModelDirectoryLocksLatestTemplateRow(t *testing.T) {
	st, db := openModelWeightStores(t)
	body := "{\"model_routes\":[{\"model\":\"shared\",\"allocations\":[{\"deployment_id\":\"gone\",\"weight\":1}]}]}"
	tmpl, err := db.CreateRouteTemplate(t.Context(), iam.Actor{Kind: "system"}, iam.TemplateOwner{}, "concurrent", body)
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Engine.NewSession()
	defer tx.Close()
	if err := tx.Begin(); err != nil {
		t.Fatal(err)
	}
	updated := "{\"retry_policy\":{\"max_attempts\":7},\"model_routes\":[{\"model\":\"shared\",\"allocations\":[{\"deployment_id\":\"gone\",\"weight\":1}]}]}"
	if _, err := tx.Exec("UPDATE route_templates SET body = ? WHERE id = ?", updated, tmpl.ID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- st.SaveModelDirectory(nil, "", []config.ModelEntry{weightDeployment("shared", "kept")})
	}()
	select {
	case err := <-done:
		_ = tx.Rollback()
		t.Fatalf("目录保存越过未提交的模板行锁: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("锁释放后目录保存失败: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("锁释放后目录保存仍未完成")
	}
	got, err := db.GetRouteTemplate(t.Context(), tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got.Body), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["retry_policy"].(map[string]any)["max_attempts"] != float64(7) {
		t.Fatalf("并发模板编辑丢失: %v", doc)
	}
	if rule := doc["model_routes"].([]any)[0].(map[string]any); rule["allocations"] != nil {
		t.Fatalf("悬空部署引用未清理: %v", rule)
	}
}
