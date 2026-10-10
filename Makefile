.PHONY: test run ui tidy testdata e2e e2e-all e2e-offline e2e-model-endpoints regression regression-live

test:
	go test ./...

# End-to-end regression suite: real gateway, real PostgreSQL, real budgets,
# guardrails and bypass. Uses a fake provider, so it is free to run repeatedly.
regression:
	./scripts/regression.sh -v

# The same suite with the live vendor calls enabled. Spends real money and needs
# XHUB_REGRESSION_<ID>_KEY in the environment.
regression-live:
	python3 scripts/with-live-vendors.py bash scripts/regression.sh -v --live

# 全量 live 回归，终端按用例实时打印。密钥从 FENNO_AI_API_KEY / QINIU_API_KEY 读取。
regression-live-log:
	bash scripts/regression-live-log.sh

run:
	go run ./cmd/gateway -config configs/config.yaml -addr :4000

ui:
	cd frontend && npm install && npm run dev

tidy:
	go mod tidy

# 清空指定 xhub/public 与 Redis DB 1，仅按清单构造数据与配置，不调用模型或供应商。
testdata:
	python3 scripts/e2e-real-dataset.py --shared-target --phase seed --directory .e2e/real-acceptance-current

# 每类路由只选一把代表，逐项打印编号/内容/结果；覆盖模型、业务浏览器、计量和后台回归，81密钥基线保留。
e2e:
	python3 scripts/e2e-real-dataset.py --shared-target --phase verify --with-regression --directory .e2e/real-acceptance-current

# 原有全套隔离验收保留为显式入口。
e2e-all:
	bash scripts/e2e-all.sh

e2e-offline:
	bash scripts/e2e.sh

e2e-model-endpoints:
	bash scripts/e2e-model-endpoints.sh
