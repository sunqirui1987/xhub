package gateway

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/store"
)

func TestBuiltinProvidersInstallOnStartup(t *testing.T) {
	cfg, st := openTestStore(t)
	// 启动行为必须从无凭据的状态验证，避免共享测试库中的旧地址影响断言。
	// 清理时恢复原行，生产逻辑仍不会覆盖操作员保存的连接。
	isolateBuiltinCredential(t, st, "fennoai")
	isolateBuiltinCredential(t, st, "qiniu")
	clearBuiltins(t, st)
	t.Cleanup(func() {
		clearBuiltins(t, st)
		rows, err := st.ListProxyModels()
		if err != nil {
			t.Fatal(err)
		}
		if row := findModelRow(rows, "extra/id"); row != nil {
			if err := st.DeleteProxyModel(row.ID); err != nil {
				t.Fatal(err)
			}
		}
	})

	var hits []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := r.Header.Get("X-Original-Host") + r.URL.Path
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && r.Header.Get("x-api-key") != "" {
			hit += " auth"
		}
		hits = append(hits, hit)
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("X-Original-Host") {
		case "api.fenno.ai":
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-fenno"},{"id":"codex-mini"}]}`)
		case "api.qnaigc.com":
			_, _ = io.WriteString(w, `{"data":[{"id":"qwen-turbo"},{"id":"deepseek/deepseek-v3.2-exp"}]}`)
		case "relay.example":
			_, _ = io.WriteString(w, `{"data":[{"id":"relay-chat"},{"id":"relay/embed"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	models.SetBuiltinClient(rewriteClient(fake.URL))
	t.Cleanup(func() { models.SetBuiltinClient(nil) })

	t.Setenv("XHUB_BUILTIN_PROVIDERS", "1")
	t.Setenv("FENNOAI_API_KEY", "fenno-test-key")
	t.Setenv("QINIU_API_KEY", "qiniu-test-key")
	_ = New(cfg, st, nil)

	rows, err := st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(builtinRows(rows, "fennoai")) != 0 || len(builtinRows(rows, "qiniu")) != 0 {
		t.Fatalf("startup stored models fenno=%v qiniu=%v", names(builtinRows(rows, "fennoai")), names(builtinRows(rows, "qiniu")))
	}
	cred, err := st.GetKV("credentials", "fennoai")
	if err != nil {
		t.Fatal(err)
	}
	values, _ := cred["credential_values"].(map[string]any)
	if values["api_base"] != "https://api.fenno.ai" {
		t.Fatal("fenno credential has incorrect origin")
	}
	qiniuCred, err := st.GetKV("credentials", "qiniu")
	if err != nil {
		t.Fatal(err)
	}
	qiniuValues, _ := qiniuCred["credential_values"].(map[string]any)
	if qiniuValues["api_base"] != "https://api.qnaigc.com" {
		t.Fatal("qiniu credential has incorrect origin")
	}
	if len(hits) != 0 {
		t.Fatalf("startup fetched a model list %#v", hits)
	}

	db := testIdentityStore(t)
	gw := httptest.NewServer(New(cfg, st, db).Handler())
	defer gw.Close()
	sess := adminSession(t, gw.URL, db)
	if err := st.PutKV("credentials", "anthropic-only", `{"credential_info":{"custom_llm_provider":"anthropic"},"credential_values":{"api_key":"wrong-protocol"}}`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteKV("credentials", "anthropic-only") })
	status, body := authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/models", []byte(`{"provider":"qiniu","credential_name":"anthropic-only"}`))
	if status != http.StatusBadRequest || !strings.Contains(string(body), "protocol") {
		t.Fatalf("incompatible credential %d %s", status, trim(body))
	}
	if len(hits) != 0 {
		t.Fatalf("incompatible credential reached provider %#v", hits)
	}
	if err := st.PutKV("credentials", "saved-relay", `{"credential_info":{"custom_llm_provider":"openai"},"credential_values":{"api_key":"server-secret","api_base":"https://relay.example/v1"}}`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteKV("credentials", "saved-relay") })
	status, body = authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/models", []byte(`{"credential_name":"saved-relay","api_key":"request-must-not-win","api_base":"https://attacker.invalid"}`))
	if status != http.StatusOK {
		t.Fatalf("generic discovery %d %s", status, trim(body))
	}
	var discovered struct {
		CredentialName string           `json:"credential_name"`
		APIBase        string           `json:"api_base"`
		Models         []map[string]any `json:"models"`
		ModelIDs       []string         `json:"model_ids"`
	}
	if err := json.Unmarshal(body, &discovered); err != nil {
		t.Fatal(err)
	}
	if discovered.CredentialName != "saved-relay" || discovered.APIBase != "https://relay.example/v1" {
		t.Fatalf("generic source %#v", discovered)
	}
	if !contains(discovered.ModelIDs, "relay-chat") || !contains(discovered.ModelIDs, "relay/embed") || len(discovered.Models) != 2 {
		t.Fatalf("generic model ids %#v models=%#v", discovered.ModelIDs, discovered.Models)
	}
	if strings.Contains(string(body), "server-secret") || strings.Contains(string(body), "request-must-not-win") {
		t.Fatalf("generic discovery exposed a secret: %s", trim(body))
	}
	if len(hits) != 1 || hits[0] != "relay.example/v1/models auth" {
		t.Fatalf("generic discovery request %#v", hits)
	}
	status, body = authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/models", []byte(`{"provider":"fennoai","credential_name":"saved-relay"}`))
	if status != http.StatusOK {
		t.Fatalf("saved provider with builtin hint %d %s", status, trim(body))
	}
	if err := json.Unmarshal(body, &discovered); err != nil {
		t.Fatal(err)
	}
	if len(discovered.Models) != 2 || !contains(discovered.ModelIDs, "relay-chat") || contains(discovered.ModelIDs, "gpt-fenno") {
		t.Fatalf("builtin hint mixed providers: %#v", discovered)
	}
	if len(hits) != 2 || hits[1] != "relay.example/v1/models auth" {
		t.Fatalf("builtin hint ignored saved provider address %#v", hits)
	}
	orphan := store.ProxyModel{ID: "provider-list-orphan", ModelName: "orphan-model", Params: map[string]any{"litellm_credential_name": "qiniu"}}
	if err := st.UpsertProxyModel(orphan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteProxyModel(orphan.ID) })
	status, body = authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/add", []byte(`{"provider":"qiniu","model_ids":["extra/id"]}`))
	if status != http.StatusGone || !strings.Contains(string(body), "/price/model") {
		t.Fatalf("retired add %d %s", status, trim(body))
	}
	rows, err = st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	if added := findModelRow(rows, "extra/id"); added != nil {
		t.Fatalf("retired endpoint still added model %#v", added)
	}

	status, body = authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/refresh", []byte(`{"provider":"qiniu"}`))
	if status != http.StatusOK {
		t.Fatalf("refresh %d %s", status, trim(body))
	}
	if strings.Contains(string(body), "orphan-model") {
		t.Fatalf("discovery appended a saved deployment absent from the provider: %s", trim(body))
	}
	rows, _ = st.ListProxyModels()
	if findModelRow(rows, "extra/id") != nil {
		t.Fatal("refresh created a model after the retired add endpoint")
	}
}

// isolateBuiltinCredential 临时移除指定内置凭据，并在用例结束后恢复原内容。
// 参数 t：测试上下文；st：共享测试存储；id：供应商凭据名。
// 返回：无；读取、删除或恢复失败时报告错误，不输出含密钥的行内容。
// 调用：TestBuiltinProvidersInstallOnStartup。测试：该启动用例。
func isolateBuiltinCredential(t *testing.T, st *store.Store, id string) {
	t.Helper()
	previous, err := st.GetKV("credentials", id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cannot read test credential")
	}
	if previous != nil {
		if err := st.DeleteKV("credentials", id); err != nil {
			t.Fatal("cannot isolate test credential")
		}
	}
	t.Cleanup(func() {
		if previous == nil {
			if err := st.DeleteKV("credentials", id); err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Error("cannot remove test credential")
			}
			return
		}
		body, err := json.Marshal(previous)
		if err != nil || st.PutKV("credentials", id, string(body)) != nil {
			t.Error("cannot restore test credential")
		}
	})
}

func TestBuiltinProvidersStayOutWhenDisabled(t *testing.T) {
	cfg, st := openTestStore(t)
	clearBuiltins(t, st)
	t.Cleanup(func() { clearBuiltins(t, st) })
	models.SetBuiltinClient(&http.Client{Transport: failTransport{t: t}})
	t.Cleanup(func() { models.SetBuiltinClient(nil) })
	t.Setenv("XHUB_BUILTIN_PROVIDERS", "off")
	t.Setenv("FENNOAI_API_KEY", "fenno-test-key")
	t.Setenv("QINIU_API_KEY", "qiniu-test-key")
	_ = New(cfg, st, nil)
	rows, err := st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(builtinRows(rows, "fennoai")) != 0 || len(builtinRows(rows, "qiniu")) != 0 {
		t.Fatalf("disabled install stored fenno=%d qiniu=%d", len(builtinRows(rows, "fennoai")), len(builtinRows(rows, "qiniu")))
	}
}

func openTestStore(t *testing.T) (*config.Config, *store.Store) {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, st
}

func clearBuiltins(t *testing.T, st *store.Store) {
	t.Helper()
	rows, err := st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		id, _ := row.Info["builtin"].(string)
		if id == "fennoai" || id == "qiniu" {
			if err := st.DeleteProxyModel(row.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func blankBuiltinKeys(t *testing.T, st *store.Store, provider string) {
	t.Helper()
	rows, err := st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Info["builtin"] != provider {
			continue
		}
		row.Params["api_key"] = ""
		if err := st.UpsertProxyModel(row); err != nil {
			t.Fatal(err)
		}
	}
}

func builtinRows(rows []store.ProxyModel, provider string) []store.ProxyModel {
	var out []store.ProxyModel
	for _, row := range rows {
		if row.Info["builtin"] == provider {
			out = append(out, row)
		}
	}
	return out
}

func roleCount(rows []store.ProxyModel, role string) int {
	n := 0
	for _, row := range rows {
		if row.Info["role"] == role {
			n++
		}
	}
	return n
}

func findModelRow(rows []store.ProxyModel, name string) *store.ProxyModel {
	for i := range rows {
		if rows[i].ModelName == name {
			return &rows[i]
		}
	}
	return nil
}

func names(rows []store.ProxyModel) []string {
	var out []string
	for _, row := range rows {
		out = append(out, row.ModelName)
	}
	return out
}

func baseOf(rows []store.ProxyModel) string {
	for _, row := range rows {
		if base, _ := row.Params["api_base"].(string); base != "" {
			return base
		}
	}
	return ""
}

func hostOf(rows []store.ProxyModel) string {
	for _, row := range rows {
		base, _ := row.Params["api_base"].(string)
		u, err := url.Parse(base)
		if err == nil && u.Host != "" {
			return u.Hostname()
		}
	}
	return ""
}

func wireOf(rows []store.ProxyModel) string {
	for _, row := range rows {
		if v, _ := row.Info["wire_api"].(string); v != "" {
			return v
		}
	}
	return ""
}

func splitStored(rows []store.ProxyModel, name string) (string, string) {
	for _, row := range rows {
		if row.ModelName != name {
			continue
		}
		raw, _ := row.Params["model"].(string)
		return config.SplitProviderModel(raw)
	}
	return "", ""
}

func keyOf(rows []store.ProxyModel, name string) string {
	for _, row := range rows {
		if row.ModelName == name {
			v, _ := row.Params["api_key"].(string)
			return v
		}
	}
	return ""
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func rewriteClient(dest string) *http.Client {
	return &http.Client{Transport: rewriteTransport{dest: dest}}
}

type rewriteTransport struct{ dest string }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(r.dest)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = u.Scheme
	clone.URL.Host = u.Host
	clone.Host = u.Host
	clone.Header = req.Header.Clone()
	clone.Header.Set("X-Original-Host", req.URL.Host)
	return http.DefaultTransport.RoundTrip(clone)
}

type swapBody struct {
	dest, fenno, qiniu string
}

func (s swapBody) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"data":[]}`
	switch req.URL.Host {
	case "api.fenno.ai":
		body = s.fenno
	case "api.qnaigc.com":
		body = s.qiniu
	}
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteString(body)
	return rec.Result(), nil
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("disabled install fetched a model list")
	return nil, nil
}
