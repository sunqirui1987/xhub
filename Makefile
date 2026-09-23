.PHONY: test run ui tidy e2e

test:
	go test ./...

run:
	go run ./cmd/gateway -config configs/config.yaml -addr :4000

ui:
	cd frontend && npm install && npm run dev

tidy:
	go mod tidy

# Browser e2e: fake upstream + gateway :4000 + Next :3000 + Playwright.
e2e:
	python3 e2e/free_ports.py
	cd frontend && npm run build
	cd frontend && npx playwright test
