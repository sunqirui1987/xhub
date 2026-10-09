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

run:
	go run ./cmd/gateway -config configs/config.yaml -addr :4000

ui:
	cd frontend && npm install && npm run dev

tidy:
	go mod tidy

# 清空指定 xhub/public 与 Redis DB 1，并保留 real-acceptance 真实数据基线。
testdata:
	python3 scripts/e2e-real-dataset.py --shared-target --phase seed --directory .e2e/real-acceptance-current

# 基于 make testdata 的保留数据执行真实浏览器、计量核对和后台回归。
e2e:
	python3 scripts/e2e-real-dataset.py --shared-target --phase verify --with-regression --directory .e2e/real-acceptance-current

# 原有全套隔离验收保留为显式入口。
e2e-all:
	bash scripts/e2e-all.sh

e2e-offline:
	bash scripts/e2e.sh

e2e-model-endpoints:
	bash scripts/e2e-model-endpoints.sh
