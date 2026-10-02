package models

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

const (
	BuiltinFenno = "fennoai"
	BuiltinQiniu = "qiniu"
)

// Builtin is a provider credential installed with the gateway. Models added from it use the same row as a hand-added model.
type Builtin struct {
	ID     string
	Base   string
	KeyEnv string
}

// Builtins are the two providers a new install can add models from.
// Fenno calls https://api.fenno.ai. Qiniu calls the OpenAI bypass root. Catalogs stay on /v1/models.
func Builtins() []Builtin {
	return []Builtin{
		{ID: BuiltinFenno, Base: "https://api.fenno.ai", KeyEnv: "FENNOAI_API_KEY"},
		{ID: BuiltinQiniu, Base: llm.QiniuBypassBase, KeyEnv: "QINIU_API_KEY"},
	}
}

// BuiltinsEnabled reports whether first install should add fennoai and qiniu.
// XHUB_BUILTIN_PROVIDERS defaults to on. 0, false, off, no, or disabled turns it off.
func BuiltinsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("XHUB_BUILTIN_PROVIDERS"))) {
	case "0", "false", "off", "no", "disable", "disabled":
		return false
	default:
		return true
	}
}

// CatalogModel is one card in the provider catalog. Prices are copied from the payload and stay nil when absent.
type CatalogModel struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	InputPrice  *float64 `json:"input_price"`
	OutputPrice *float64 `json:"output_price"`
	Added       bool     `json:"added"`
}

// ParseModelIDs reads an OpenAI models list. A body with no ids returns an empty slice.
func ParseModelIDs(body []byte) []string {
	items := ParseCatalog(body)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// ParseCatalog maps an OpenAI-style models document to catalog cards.
func ParseCatalog(body []byte) []CatalogModel {
	rows := catalogObjects(body)
	seen := map[string]bool{}
	var items []CatalogModel
	for _, row := range rows {
		id := strings.TrimSpace(str(row["id"]))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		items = append(items, CatalogModel{
			ID:          id,
			Category:    catalogCategory(id, row),
			InputPrice:  catalogPrice(row, "input_price", "input_cost_per_token", "input", "prompt"),
			OutputPrice: catalogPrice(row, "output_price", "output_cost_per_token", "output", "completion"),
		})
	}
	return items
}

func catalogObjects(body []byte) []map[string]any {
	var doc struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Data == nil {
		var bare []map[string]any
		if json.Unmarshal(body, &bare) != nil {
			return nil
		}
		return bare
	}
	return doc.Data
}

func catalogCategory(id string, row map[string]any) string {
	for _, key := range []string{"category", "type", "modality"} {
		if value := strings.TrimSpace(str(row[key])); value != "" && value != "model" {
			return value
		}
	}
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "embed"):
		return "embedding"
	case strings.Contains(lower, "vision") || strings.Contains(lower, "-vl"):
		return "vision"
	case strings.Contains(lower, "whisper") || strings.Contains(lower, "tts") || strings.Contains(lower, "audio"):
		return "audio"
	}
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return "llm"
}

func catalogPrice(row map[string]any, keys ...string) *float64 {
	if price := firstNumber(row, keys...); price != nil {
		return price
	}
	pricing, _ := row["pricing"].(map[string]any)
	if pricing == nil {
		return nil
	}
	return firstNumber(pricing, keys...)
}

func firstNumber(row map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if number := asFloat(value); number != nil {
				return number
			}
		}
	}
	return nil
}

func asFloat(value any) *float64 {
	switch number := value.(type) {
	case float64:
		return &number
	case json.Number:
		parsed, err := number.Float64()
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

// ModelsURL is the provider model catalog. These URLs are fixed and do not follow a rewritten api_base.
// Fenno's catalog is https://api.fenno.ai/v1/models and requires the credential key.
// Qiniu's catalog is https://api.qnaigc.com/v1/models and is public.
func ModelsURL(provider, base string) string {
	if provider == BuiltinFenno {
		return "https://api.fenno.ai/v1/models"
	}
	if provider == BuiltinQiniu {
		return "https://api.qnaigc.com/v1/models"
	}
	return strings.TrimRight(strings.TrimSpace(base), "/") + "/models"
}

// builtinClient is the HTTP client that fetches provider model lists. Tests replace it.
var builtinClient = http.DefaultClient

// SetBuiltinClient replaces the client used to fetch builtin model lists.
func SetBuiltinClient(c *http.Client) {
	if c == nil {
		builtinClient = http.DefaultClient
		return
	}
	builtinClient = c
}

// SeedBuiltins removes leftover provider rows and creates the two credentials when they are missing.
// It does not insert models. A model added later is the same row a person would save by hand.
func SeedBuiltins(s Host) {
	if s == nil || s.DB() == nil || !BuiltinsEnabled() {
		return
	}
	dropProviderShells(s)
	for _, spec := range Builtins() {
		seedCredential(s, spec, strings.TrimSpace(os.Getenv(spec.KeyEnv)))
	}
}

// dropProviderShells deletes fennoai and qiniu rows that were stored as models.
// They are credentials. Leaving them in the model table makes the playground list the provider name.
func dropProviderShells(s Host) {
	rows, err := s.DB().ListProxyModels()
	if err != nil {
		logx.Error("builtin providers list: %v", err)
		return
	}
	for _, row := range rows {
		if str(row.Info["role"]) != "provider" {
			continue
		}
		if _, ok := builtinByID(str(row.Info["builtin"])); !ok {
			continue
		}
		if err := s.DB().DeleteProxyModel(row.ID); err != nil {
			logx.Error("builtin provider %s: %v", row.ID, err)
		}
	}
}

// RefreshBuiltin reloads the catalog. It does not add or delete models.
func RefreshBuiltin(s Host, w http.ResponseWriter, r *http.Request) {
	ListBuiltin(s, w, r)
}

// ListBuiltin returns the provider catalog and which names are already saved. It does not write.
func ListBuiltin(s Host, w http.ResponseWriter, r *http.Request) {
	spec, rows, key, _, ok := openBuiltin(s, w, r)
	if !ok {
		return
	}
	already := savedNames(rows, spec.ID)
	var available []CatalogModel
	var fetchErr string
	items, err := fetchCatalog(ModelsURL(spec.ID, spec.Base), key)
	if err != nil {
		fetchErr = err.Error()
	} else {
		available = items
	}
	if available == nil {
		available = []CatalogModel{}
	}
	seen := map[string]bool{}
	models := []map[string]any{}
	for _, item := range available {
		seen[item.ID] = true
		fillFromCostMap(&item)
		item.Added = already[item.ID]
		models = append(models, catalogJSON(item))
	}
	for id := range already {
		if seen[id] {
			continue
		}
		models = append(models, catalogJSON(CatalogModel{ID: id, Category: catalogCategory(id, nil), Added: true}))
	}
	out := map[string]any{"provider": spec.ID, "api_base": spec.Base, "models": models}
	if fetchErr != "" {
		out["error"] = fetchErr
	}
	httpx.WriteJSON(w, 200, out)
}

// AddBuiltinModels saves each selected name the same way the model form does.
// The row stores the model name and the provider credential. The address and key stay on the credential.
func AddBuiltinModels(s Host, w http.ResponseWriter, r *http.Request) {
	spec, rows, _, body, ok := openBuiltin(s, w, r)
	if !ok {
		return
	}
	var ids []string
	switch listed := body["model_ids"].(type) {
	case []any:
		for _, item := range listed {
			if id := strings.TrimSpace(str(item)); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		httpx.WriteJSON(w, 200, map[string]any{"provider": spec.ID, "updated": false, "api_base": spec.Base, "model_ids": ids})
		return
	}
	already := savedNames(rows, spec.ID)
	s.LockModels()
	defer s.UnlockModels()
	saved := []string{}
	for _, id := range ids {
		if already[id] {
			continue
		}
		entry := addedModel(spec.ID, id)
		if err := s.DB().UpsertProxyModel(proxyModel(entry)); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
		*s.ModelTable() = append(*s.ModelTable(), entry)
		saved = append(saved, id)
	}
	httpx.WriteJSON(w, 200, map[string]any{"provider": spec.ID, "updated": len(saved) > 0, "api_base": spec.Base, "model_ids": saved})
}

// addedModel is one catalog id stored like a model typed into the form.
func addedModel(provider, name string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"model":                   name,
			"custom_llm_provider":     "openai",
			"litellm_credential_name": provider,
		},
		ModelInfo: map[string]any{
			"id":         "model_" + httpx.CallID()[:12],
			"db_model":   true,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
}

func openBuiltin(s Host, w http.ResponseWriter, r *http.Request) (Builtin, []store.ProxyModel, string, map[string]any, bool) {
	if s.RequireManage(w, r) == nil {
		return Builtin{}, nil, "", nil, false
	}
	body := readBody(r)
	spec, found := builtinByID(strings.TrimSpace(str(body["provider"])))
	if !found {
		httpx.WriteError(w, 400, "invalid_request", "provider must be fennoai or qiniu")
		return Builtin{}, nil, "", nil, false
	}
	rows, err := s.DB().ListProxyModels()
	if err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return Builtin{}, nil, "", nil, false
	}
	return spec, rows, providerKey(s, spec, str(body["api_key"])), body, true
}

func readBody(r *http.Request) map[string]any {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// providerKey is the key used only to read the catalog. The saved model does not copy it.
func providerKey(s Host, spec Builtin, requestKey string) string {
	if key := realKey(requestKey); key != "" {
		return key
	}
	if key := credentialAPIKey(s, spec.ID); key != "" {
		return key
	}
	return realKey(os.Getenv(spec.KeyEnv))
}

func realKey(raw string) string {
	key := strings.TrimSpace(raw)
	if key == "" || strings.Contains(key, "****") {
		return ""
	}
	return key
}

func credentialAPIKey(s Host, id string) string {
	if s.DB() == nil {
		return ""
	}
	rec, err := s.DB().GetKV("credentials", id)
	if err != nil {
		return ""
	}
	values, _ := rec["credential_values"].(map[string]any)
	return strings.TrimSpace(str(values["api_key"]))
}

func fetchCatalog(rawURL, key string) ([]CatalogModel, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	}
	resp, err := builtinClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, errString(resp.Status)
	}
	return ParseCatalog(body), nil
}

// fillFromCostMap copies input/output prices from the price-data map when the catalog payload omitted them.
// The stored rates are per token; the card shows the same per-million-token dollars as Price Data Management.
func fillFromCostMap(item *CatalogModel) {
	if item.InputPrice != nil && item.OutputPrice != nil {
		return
	}
	row := costRow(item.ID)
	if row == nil {
		return
	}
	if item.InputPrice == nil {
		item.InputPrice = perMillion(row["input_cost_per_token"])
	}
	if item.OutputPrice == nil {
		item.OutputPrice = perMillion(row["output_cost_per_token"])
	}
}

func costRow(id string) map[string]any {
	prices := catalog.CostMap()
	if row := prices[id]; row != nil {
		return row
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if row := prices[id[i+1:]]; row != nil {
			return row
		}
	}
	return nil
}

func perMillion(value any) *float64 {
	rate, ok := value.(float64)
	if !ok {
		return nil
	}
	scaled := rate * 1_000_000
	return &scaled
}

func catalogJSON(item CatalogModel) map[string]any {
	return map[string]any{
		"id":           item.ID,
		"category":     item.Category,
		"input_price":  item.InputPrice,
		"output_price": item.OutputPrice,
		"added":        item.Added,
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func builtinByID(id string) (Builtin, bool) {
	for _, b := range Builtins() {
		if b.ID == id {
			return b, true
		}
	}
	return Builtin{}, false
}

// savedNames reports catalog ids already stored for this credential, including older builtin rows.
func savedNames(rows []store.ProxyModel, provider string) map[string]bool {
	out := map[string]bool{}
	for _, row := range rows {
		if str(row.Info["role"]) == "provider" {
			continue
		}
		if str(row.Params["litellm_credential_name"]) == provider && row.ModelName != "" {
			out[row.ModelName] = true
		}
		if str(row.Info["builtin"]) == provider {
			if id := str(row.Info["upstream_id"]); id != "" {
				out[id] = true
			}
		}
	}
	return out
}

// clearCopiedMode removes a mode that older imports copied onto the model.
// A hand-added model has no mode, so the playground treats it as chat.
func clearCopiedMode(row *store.ProxyModel) bool {
	if row.Info == nil || str(row.Info["builtin"]) == "" {
		return false
	}
	mode := str(row.Info["mode"])
	if mode != "chat" && mode != "responses" {
		return false
	}
	delete(row.Info, "mode")
	return true
}

func seedCredential(s Host, spec Builtin, key string) {
	if _, err := s.DB().GetKV("credentials", spec.ID); err == nil {
		return
	}
	raw, err := json.Marshal(map[string]any{
		"credential_name": spec.ID,
		"credential_info": map[string]any{
			"custom_llm_provider": "openai",
			"builtin":             spec.ID,
			"api_base":            spec.Base,
		},
		"credential_values": map[string]any{
			"api_key":  key,
			"api_base": spec.Base,
		},
	})
	if err != nil {
		return
	}
	if err := s.DB().PutKV("credentials", spec.ID, string(raw)); err != nil {
		logx.Error("builtin credential %s: %v", spec.ID, err)
	}
}
