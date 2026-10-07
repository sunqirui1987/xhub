.PHONY: test run ui tidy e2e regression regression-live

test:
	go test ./...

# End-to-end regression suite: real gateway, real PostgreSQL, real budgets,
# guardrails and bypass. Uses a fake provider, so it is free to run repeatedly.
regression:
	./scripts/regression.sh -v

# The same suite with the live vendor calls enabled. Spends real money and needs
# XHUB_REGRESSION_FENNO_KEY and XHUB_REGRESSION_QINIU_KEY in the environment.
regression-live:
	./scripts/regression.sh -v --live

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

# Browser e2e: fake upstream + gateway :4000 + Next :3000 + Playwright.
e2e:
	python3 e2e/free_ports.py
	cd frontend && npm run build
	python3 e2e/free_ports.py
	cd frontend && npx playwright test
