.PHONY: test run ui tidy e2e e2e-all e2e-offline e2e-model-endpoints regression regression-live

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

# Optional verification tenant. Not part of `make run`.
seed:
	go run ./cmd/seed -config configs/config.yaml

verify-seed:
	go run ./cmd/seed -config configs/config.yaml -verify -gateway http://127.0.0.1:4000

ui:
	cd frontend && npm install && npm run dev

tidy:
	go mod tidy

# Full acceptance includes actual selected suppliers.
e2e:
	bash scripts/e2e-all.sh

e2e-all: e2e

e2e-offline:
	bash scripts/e2e.sh

e2e-model-endpoints:
	bash scripts/e2e-model-endpoints.sh
