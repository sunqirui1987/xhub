package gateway

import (
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
		t.Fatalf("fenno credential %#v", values)
	}
	qiniuCred, err := st.GetKV("credentials", "qiniu")
	if err != nil {
		t.Fatal(err)
	}
	qiniuValues, _ := qiniuCred["credential_values"].(map[string]any)
	if qiniuValues["api_base"] != "https://api.qnaigc.com/bypass/openai/v1" {
		t.Fatalf("qiniu credential %#v", qiniuValues)
	}
	if len(hits) != 0 {
		t.Fatalf("startup fetched a model list %#v", hits)
	}

	db := testIdentityStore(t)
	gw := httptest.NewServer(New(cfg, st, db).Handler())
	defer gw.Close()
	sess := adminSession(t, gw.URL, db)
	status, body := authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/add", []byte(`{"provider":"qiniu","model_ids":["extra/id"]}`))
	if status != http.StatusOK {
		t.Fatalf("add %d %s", status, trim(body))
	}
	rows, err = st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	added := findModelRow(rows, "extra/id")
	if added == nil {
		t.Fatalf("add missed model %#v", names(rows))
	}
	if added.Params["model"] != "extra/id" || added.Params["custom_llm_provider"] != "openai" || added.Params["litellm_credential_name"] != "qiniu" {
		t.Fatalf("params %#v", added.Params)
	}
	if _, ok := added.Params["api_base"]; ok || added.Info["mode"] != nil || added.Info["role"] != nil {
		t.Fatalf("row is not a plain model params=%#v info=%#v", added.Params, added.Info)
	}

	status, body = authed(t, gw.URL, sess, http.MethodPost, "/model/builtin/refresh", []byte(`{"provider":"qiniu"}`))
	if status != http.StatusOK {
		t.Fatalf("refresh %d %s", status, trim(body))
	}
	rows, _ = st.ListProxyModels()
	if findModelRow(rows, "extra/id") == nil {
		t.Fatal("refresh deleted the added model")
	}
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
