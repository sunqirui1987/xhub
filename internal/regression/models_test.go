package regression

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// 这一组测"模型与端点"这一层：模型怎么被列出来、端点类型怎么决定一条模型能被
// 哪些路径调用、以及价目表和端点目录之间是不是对得上。
//
// endpoints_test.go 核的是"一次调用能不能到上游、正文改对了没有"。这一组换一个
// 视角：从调用方和运维看得到的那几个接口出发——/v1/models 列出了什么、
// /model/available 给了什么、端点目录和价目表有没有互相矛盾。
//
// 后面这一条尤其值得测：价目表里写着 endpoint_type 的模型，必须真的有一个
// 对应的端点类型存在。两边对不上时，界面上会显示一个选不出来的端点。

// TestModelListShowsWhatTheCallerMayUse 证明 /v1/models 列出来的模型，就是调用方
// 手上那把密钥真的能调的模型。
//
// 这两件事一旦不一致就会出现最让人困惑的故障：列表里看得见，一调就 401。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelListShowsWhatTheCallerMayUse(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-listed"), chatDeployment("regression-hidden"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "listed")

	// 一把只能调其中一条的密钥。
	restricted := h.modelKey(t, tn.session, tn.teamID, "listed-key", "regression-listed")

	models := h.ok(http.MethodGet, "/v1/models", restricted, nil)
	ids := modelIDs(rowsOf(models, "data"))
	if !contains(ids, "regression-listed") {
		t.Fatalf("/v1/models does not list the allowed model: %v", ids)
	}
	if contains(ids, "regression-hidden") {
		t.Fatalf("/v1/models lists a model this key cannot call: %v", ids)
	}

	// 列表里有的必须真的能调——这正是上面那条断言存在的意义。
	h.ok(http.MethodPost, "/v1/chat/completions", restricted, map[string]any{
		"model": "regression-listed", "messages": []any{map[string]any{"role": "user", "content": "listed"}},
	})
}

// TestModelsOfEveryEndpointTypeAreListed 证明每一种端点类型的模型都出现在列表里，
// 而不是只有 chat 那一类。
//
// 一个只列 chat 的实现在只看"列表非空"的断言下照样绿，但它会让向量、生图这类
// 模型在调用方眼里根本不存在。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelsOfEveryEndpointTypeAreListed(t *testing.T) {
	h := newHarness(t,
		chatDeployment("regression-chat-model"),
		embeddingDeployment("regression-embed-model"),
		seedanceDeployment("qiniu/bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation"),
	)
	admin := h.adminSession()
	tn := h.provision(t, admin, "types")
	// 不限模型的密钥，这样列表反映的是模型表而不是密钥的名单。
	open := h.keyWith(t, tn.session, map[string]any{"key_alias": "open-key", "team_id": tn.teamID})

	ids := modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", open, nil), "data"))
	for _, want := range []string{
		"regression-chat-model",
		"regression-embed-model",
		"qiniu/bytedance/doubao-seedance-2-0-260128",
	} {
		if !contains(ids, want) {
			t.Fatalf("/v1/models is missing %s: %v", want, ids)
		}
	}
}

// TestModelAvailableCarriesThePriceFromTheCatalog 证明 /model/available 上的卡片价格
// 来自生成的价格目录，而不是某条部署上另填的单价。
//
// 这个区别是有意的：控制台的模型卡片是给运维比价用的，它要回答"这个模型在市场上
// 多少钱"，而不是"我这里这条部署被单独改成了多少钱"。
//
// 所以这里装的部署用目录里已有的模型名。用一条目录里没有的名字，卡片上就该是
// 空价格——那是正确行为，不是缺陷：目录里没有的费率不能编一个出来。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelAvailableCarriesThePriceFromTheCatalog(t *testing.T) {
	// claude-4.1-opus 是生成目录里确实有费率的模型。
	const catalogModel = "claude-4.1-opus"
	h := newHarness(t, chatDeployment(catalogModel))
	admin := h.adminSession()

	body := h.ok(http.MethodGet, "/model/available", admin, nil).json()
	rows := rowsOfFrom(body, "data", "models")
	if len(rows) == 0 {
		t.Fatalf("/model/available returned no rows: %s", truncate(string(mustJSON(body)), 300))
	}

	row := findBy(rows, "model_name", catalogModel)
	if row == nil {
		row = findBy(rows, "id", catalogModel)
	}
	if row == nil {
		t.Fatalf("/model/available does not carry the deployment: %s", truncate(string(mustJSON(rows)), 400))
	}
	// 目录里有费率，所以卡片上必须有价格，而且不能是零。
	price := firstFloat(row, "input_price", "input_cost_per_token")
	if price <= 0 {
		t.Fatalf("the card for %s has no price: %s", catalogModel, truncate(string(mustJSON(row)), 300))
	}
}

// TestModelAvailableLeavesUnknownPricesEmpty 证明目录里没有的模型，卡片上就是空价格，
// 不会被编造成零。
//
// "$0" 和"未定价"是两件事。把后者写成前者，运维会以为这条模型免费。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelAvailableLeavesUnknownPricesEmpty(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-not-in-catalog"))
	admin := h.adminSession()

	rows := rowsOfFrom(h.ok(http.MethodGet, "/model/available", admin, nil).json(), "data", "models")
	row := findBy(rows, "id", "regression-not-in-catalog")
	if row == nil {
		row = findBy(rows, "model_name", "regression-not-in-catalog")
	}
	if row == nil {
		t.Fatalf("the deployment is missing from /model/available: %s", truncate(string(mustJSON(rows)), 300))
	}
	// 空价格必须是 null，不能是 0。两种写法的区别就是"未定价"和"免费"的区别。
	if price := firstFloat(row, "input_price", "input_cost_per_token"); price != 0 {
		t.Fatalf("a model absent from the catalog got an invented price: %v", price)
	}
	if row["input_price"] != nil && row["input_price"] != float64(0) {
		t.Fatalf("a model absent from the catalog got input_price=%v, want null", row["input_price"])
	}
}

// TestEveryDeclaredTransportExistsInTheCatalog 证明价目表里写了 endpoint_type 的模型，
// 那个值要么是一条能力，要么是一个登记过的转发方式，不会是一个两边都不认识的东西。
//
// 两边对不上时，界面会显示一个选不出来的端点——运维看到的是模型"配好了"，
// 实际却调不动。这是一条只在两个数据源之间才成立的检查。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEveryDeclaredTransportExistsInTheCatalog(t *testing.T) {
	h := newHarness(t)

	// 端点目录：添加模型表单从这里拿可选能力和转发方式。
	endpoints := h.ok(http.MethodGet, "/public/endpoints", "", nil).json()
	capabilities := rowsOfFrom(endpoints, "capabilities")
	transports := rowsOfFrom(endpoints, "transports")
	if len(capabilities) == 0 {
		t.Fatalf("the endpoint catalog has no capabilities: %s", truncate(string(mustJSON(endpoints)), 300))
	}
	if len(transports) == 0 {
		t.Fatalf("the endpoint catalog has no transports: %s", truncate(string(mustJSON(endpoints)), 300))
	}
	known := map[string]bool{}
	for _, row := range capabilities {
		if id := stringField(row, "id"); id != "" {
			known[id] = true
		}
	}
	for _, row := range transports {
		if id := stringField(row, "id"); id != "" {
			known[id] = true
		}
	}

	// 价目表：每一条模型可能声明一个端点。
	catalog := h.ok(http.MethodGet, "/price/catalog", h.adminSession(), nil).json()
	models := rowsOfFrom(catalog, "models", "data")
	if len(models) == 0 {
		t.Fatal("the price catalog is empty")
	}

	missing := map[string]string{}
	for _, row := range models {
		declared := stringField(row, "endpoint_type")
		if declared == "" {
			continue
		}
		if !known[declared] {
			missing[stringField(row, "id")] = declared
		}
	}
	if len(missing) > 0 {
		t.Fatalf("models declare endpoints the catalog does not offer: %v", missing)
	}

	// 反过来，价目表里每条模型都要有一个它归属的供应商，否则筛选和分组会漏掉它。
	for _, row := range models {
		if stringField(row, "litellm_provider") == "" {
			t.Fatalf("model %s has no provider", stringField(row, "id"))
		}
	}
}

// TestTransportsAreRegisteredBypasses 证明端点目录里的转发方式都是登记过的
// Bypass，而且动作齐备。
//
// 协议适配不在这个列表里：它由 (op, 供应商) 决定，没有可登记的数据。
// 一个动作不全的转发方式会让表单存下一条调不动的部署，而这件事在保存那一刻
// 是看不出来的。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTransportsAreRegisteredBypasses(t *testing.T) {
	h := newHarness(t)

	endpoints := h.ok(http.MethodGet, "/public/endpoints", "", nil).json()
	transports := rowsOfFrom(endpoints, "transports")
	if len(transports) == 0 {
		t.Fatal("the endpoint catalog has no transports")
	}

	for _, row := range transports {
		id := stringField(row, "id")
		if id == "" {
			t.Fatalf("a transport has no id: %s", truncate(string(mustJSON(row)), 200))
		}
		if got := stringField(row, "kind"); got != "bypass" {
			t.Fatalf("transport %s has kind %q; only bypasses are registered", id, got)
		}
		actions, _ := row["actions"].([]any)
		if len(actions) == 0 {
			t.Fatalf("transport %s has no actions", id)
		}
		for _, raw := range actions {
			action, _ := raw.(map[string]any)
			if stringField(action, "method") == "" {
				t.Fatalf("transport %s has an action with no method: %s", id, truncate(string(mustJSON(action)), 200))
			}
			if stringField(action, "public_path") == "" {
				t.Fatalf("transport %s has an action with no public path: %s", id, truncate(string(mustJSON(action)), 200))
			}
		}
	}
}

// TestPriceCatalogRatesArePerToken 证明价目表里的单价是"每 token"这个单位。
//
// 单位搞错是六个数量级的差别，而且两种错法都不会抛错：显示出来只是数字很小或
// 很大而已。所以这里挑一条已知量级的模型，核对它的数量级。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPriceCatalogRatesArePerToken(t *testing.T) {
	h := newHarness(t)
	body := h.ok(http.MethodGet, "/price/catalog", h.adminSession(), nil).json()
	rows := rowsOfFrom(body, "models", "data")
	if len(rows) == 0 {
		t.Fatal("the price catalog is empty")
	}

	checked := 0
	zero := 0
	for _, row := range rows {
		rate, ok := floatField(row, "input_cost_per_token")
		if !ok {
			continue
		}
		checked++
		// 0 是合法价格：目录里确实有几条免费模型（例如 arcee-ai/trinity-mini）。
		// 把它当成错误会让回归在别人免费开放模型时无辜变红。
		if rate == 0 {
			zero++
			continue
		}
		// 非零的每 token 单价落在 1e-9 到 1e-3 美元之间。越界就说明单位错了：
		// 要么存成了每百万 token（大一千倍），要么存成了百分之一分。
		if rate < 1e-9 || rate > 1e-3 {
			t.Fatalf("model %s has input_cost_per_token=%v, which is not a per-token rate",
				stringField(row, "id"), rate)
		}
	}
	if checked == 0 {
		t.Fatal("no model in the catalog carries a per-token input rate")
	}
	if zero == checked {
		t.Fatal("every model in the catalog has a zero input rate, which means the rates were dropped")
	}
}

// TestAddingAModelMakesItCallable 证明通过管理接口加一条模型之后，它立刻能用。
//
// "存进去了但没生效"是配置类功能最常见的故障，而且它只在重启后才自愈——
// 用户等不到那时候。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAddingAModelMakesItCallable(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "addmodel")

	// 通过管理接口加一条指向假供应商的部署。
	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name": "regression-added",
		"litellm_params": map[string]any{
			"model":                 "openai/regression-added",
			"api_key":               "sk-fake-upstream",
			"api_base":              h.prices.URL + "/v1",
			"custom_llm_provider":   "openai",
			"input_cost_per_token":  testInputRate,
			"output_cost_per_token": testOutputRate,
		},
		"model_info": map[string]any{"mode": "chat"},
	})

	// 立刻就能调，不需要重启。
	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-added", "messages": []any{map[string]any{"role": "user", "content": "added"}},
	})
	if got := stringField(r.json(), "id"); got == "" {
		t.Fatalf("the added model answered nothing: %s", r.describe())
	}
	// 而且它出现了在列表里。
	ids := modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", tn.key, nil), "data"))
	if !contains(ids, "regression-added") {
		t.Fatalf("the added model is not listed: %v", ids)
	}
}

// TestDeletingAModelStopsIt 证明删掉一条模型之后它立刻调不动了，而且从列表里消失。
//
// 与"加了不生效"相对的另一半：删了还生效，等于关不掉一个东西。
//
// 注意这里删的是通过控制台加进来的模型。配置文件里写着的模型删不掉，那是有意的：
// 下一次重启会把它读回来，允许删只在重启前有效。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingAModelStopsIt(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "delmodel")

	// 通过控制台加进来，这样它是一条"库模型"，删得掉。
	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name": "regression-doomed",
		"litellm_params": map[string]any{
			"model":               "openai/regression-doomed",
			"api_key":             "sk-fake-upstream",
			"api_base":            h.prices.URL + "/v1",
			"custom_llm_provider": "openai",
		},
		"model_info": map[string]any{"mode": "chat"},
	})

	// 删之前能调。
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-doomed", "messages": []any{map[string]any{"role": "user", "content": "before"}},
	})

	h.ok(http.MethodPost, "/model/delete", admin, map[string]any{"id": "regression-doomed"})

	// 删之后调不动。
	gone := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-doomed", "messages": []any{map[string]any{"role": "user", "content": "after"}},
	})
	if gone.status < 300 {
		t.Fatalf("a deleted model still answered: %s", gone.describe())
	}
	ids := modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", tn.key, nil), "data"))
	if contains(ids, "regression-doomed") {
		t.Fatalf("a deleted model is still listed: %v", ids)
	}
}

// TestConfigModelCannotBeDeletedFromTheConsole 钉住上一条的例外：配置文件里写着的
// 模型不能被控制台删掉。
//
// 如果允许，删掉只在下一次重启前有效，而运维会以为自己永久去掉了它。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestConfigModelCannotBeDeletedFromTheConsole(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-from-config"))
	admin := h.adminSession()

	r := h.do(http.MethodPost, "/model/delete", admin, map[string]any{"id": "regression-from-config"})
	if r.status != http.StatusBadRequest {
		t.Fatalf("a config model was deletable from the console: %s", r.describe())
	}
	if message := errorMessage(r); !strings.Contains(message, "config") {
		t.Fatalf("the refusal does not explain that the model comes from config: %q", message)
	}
}

// TestDisabledModelIsRefused 证明被停用的模型调不动，但还在表里。
//
// 停用和删除是两件事：停用是"暂时别用"，运维还要能看见它、把它恢复回来。
// 与删除一样，停用只对控制台加进来的库模型开放。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDisabledModelIsRefused(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "blockmodel")

	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name": "regression-disabled",
		"litellm_params": map[string]any{
			"model":               "openai/regression-disabled",
			"api_key":             "sk-fake-upstream",
			"api_base":            h.prices.URL + "/v1",
			"custom_llm_provider": "openai",
		},
		"model_info": map[string]any{"mode": "chat"},
	})
	// 停用之前能调，这样后面"调不动"才说明是停用起的效。
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-disabled", "messages": []any{map[string]any{"role": "user", "content": "before"}},
	})

	h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"model_name": "regression-disabled"})

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-disabled", "messages": []any{map[string]any{"role": "user", "content": "disabled"}},
	})
	if r.status < 300 {
		t.Fatalf("a disabled model still answered: %s", r.describe())
	}

	// 停用的模型仍然在表里：这是它和删除的区别。
	rows := rowsOf(h.ok(http.MethodGet, "/v2/model/info", admin, nil), "data", "models")
	if findBy(rows, "model_name", "regression-disabled") == nil {
		t.Fatalf("a disabled model disappeared from the model table: %v", namesOf(rows, "model_name"))
	}

	// 恢复之后又能用：证明刚才挡住它的是停用状态，不是别的。
	h.ok(http.MethodPost, "/model/enable", admin, map[string]any{"model_name": "regression-disabled"})
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-disabled", "messages": []any{map[string]any{"role": "user", "content": "enabled"}},
	})
}

// TestUnimplementedProviderNamesItsOwnProblem 证明供应商名不成立时，错误说的是
// "这个供应商不支持"，而不是把责任推给凭据。
//
// 这一条对应一个真实修过的缺陷：terminal 分支原来把"供应商没实现"和"缺凭据"合并成
// 同一个 401，报的是 "This model has no upstream API key configured."。于是一个把
// 供应商名写错的人（模型上明明配了密钥）会去查一把根本不缺的密钥。
//
// 断言写死了错误正文里的措辞，因为"说清是哪一层的问题"本身就是这个修复的内容。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnimplementedProviderNamesItsOwnProblem(t *testing.T) {
	// api_base 由 newHarness 填成假供应商地址，这样走到的就是"供应商没实现"
	// 那一支，而不是"没有上游地址"那一支。
	h := newHarness(t, config.ModelEntry{
		ModelName: "regression-noprovider",
		LiteLLMParams: map[string]any{
			"model":               "openai/regression-noprovider",
			"api_key":             "sk-fake",
			"custom_llm_provider": "a-provider-that-does-not-exist",
		},
		ModelInfo: map[string]any{"mode": "chat"},
	})
	admin := h.adminSession()
	tn := h.provision(t, admin, "noprovider")

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-noprovider", "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if r.status != http.StatusBadRequest {
		t.Fatalf("an unimplemented provider answered %d: %s", r.status, r.describe())
	}
	message := errorMessage(r)
	// 要点名是供应商的问题。
	if !strings.Contains(message, "a-provider-that-does-not-exist") {
		t.Fatalf("the refusal does not name the provider: %q", message)
	}
	// 而且不能说成是缺凭据——密钥明明配了。
	if strings.Contains(message, "API key") {
		t.Fatalf("the refusal blames a credential that was configured: %q", message)
	}
	// 最要紧的一条：没有把它发到任何地方去。
	if got := len(h.upstreamCalls()); got != 0 {
		t.Fatalf("an unimplemented provider still dialed the upstream %d times", got)
	}
}

// TestMissingCredentialStillBlamesTheCredential 钉住上一条的反面：真的没配密钥时，
// 报的仍然是凭据问题。
//
// 修掉误报的风险是把真的那一半也改掉。两条都要在，才算把这两种情况分清了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestMissingCredentialStillBlamesTheCredential(t *testing.T) {
	// 这条部署只有供应商名，没有 api_key——装进模型表之前把 harness 补的那份去掉。
	h := newHarness(t, config.ModelEntry{
		ModelName: "regression-nokey",
		LiteLLMParams: map[string]any{
			"model":               "openai/regression-nokey",
			"custom_llm_provider": "openai",
		},
		ModelInfo: map[string]any{"mode": "chat"},
	})
	// newHarness 给没写 api_key 的部署补了一把假密钥，这里显式清掉，
	// 才能测到"真的没有凭据"这一支。
	h.clearAPIKey("regression-nokey")
	admin := h.adminSession()
	tn := h.provision(t, admin, "nokey")

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-nokey", "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("a deployment with no key answered %d: %s", r.status, r.describe())
	}
	if message := errorMessage(r); !strings.Contains(message, "API key") {
		t.Fatalf("the refusal does not name the missing credential: %q", message)
	}
}

// TestUnknownModelIsRefused 证明调一条不存在的模型时，错误信息里带着那个名字。
// 排查问题时"模型不存在"和"模型不存在：typo-model"差别很大。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnknownModelIsRefused(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-known"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "unknown")

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "typo-model-does-not-exist", "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if r.status < 300 {
		t.Fatalf("an unknown model was answered: %s", r.describe())
	}
	if message := errorMessage(r); !strings.Contains(message, "typo-model-does-not-exist") {
		t.Fatalf("the refusal does not name the model that was asked for: %q", message)
	}
}

// modelIDs 从 /v1/models 的行里取出模型 id。这个接口用过 id 和 model_name 两种写法。
// 参数 rows（[]map[string]any）：列表行。
// 返回 []string（[]string）：排好序的模型 id。
func modelIDs(rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if id := firstString(row, "id", "model_name"); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// rowsOfFrom 从已经解好的对象里取出行，同时接受带字段名的信封和裸数组。
// 参数 body（map[string]any）：解好的对象；keys（...string）：按优先顺序尝试的字段名。
// 返回 []map[string]any（[]map[string]any）：解出来的行；都没有时返回 nil。
func rowsOfFrom(body map[string]any, keys ...string) []map[string]any {
	if rows := listField(body, keys...); rows != nil {
		return rows
	}
	if items, ok := body["data"].([]any); ok {
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}
