.PHONY: test run ui tidy e2e

test:
	go test ./...

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
