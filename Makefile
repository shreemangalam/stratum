.PHONY: setup dev test test-go test-web test-e2e gen lint lint-go lint-web clean

# --- Setup ---

setup:
	git config core.hooksPath .githooks
	git config core.autocrlf false
	cd server && go mod download
	cd web && npm install
	@echo "Setup complete."

# --- Development ---

dev:
	docker compose up -d
	@echo "Postgres running."
	@echo "Start the server:  cd server && go run ./cmd/api"
	@echo "Start the frontend: cd web && npm run dev"

# --- Testing ---

test: test-go test-web

test-go:
	cd server && go test ./...

test-web:
	cd web && npm run build

test-e2e:
	cd web && npm run test:e2e

# --- Code generation ---

gen:
	@echo "Regenerating from OpenAPI spec..."
	oapi-codegen -generate types -package generated \
		-o server/internal/http/generated/types.gen.go \
		contract/openapi.yaml
	cd web && npx openapi-typescript ../contract/openapi.yaml \
		-o lib/api/generated/schema.ts

# --- Linting ---

lint: lint-go lint-web

lint-go:
	cd server && gofumpt -l -w .
	cd server && golangci-lint run ./...

lint-web:
	cd web && npx biome check .

# --- Clean ---

clean:
	cd web && rm -rf .next node_modules
	docker compose down -v
