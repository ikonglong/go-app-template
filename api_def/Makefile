# Makefile for Medeo API Specs

.PHONY: install lint mock merge up down logs clean

install: ## Install required tools
	npm install -g @stoplight/prism-cli @stoplight/spectral-cli openapi-merge-cli

# Disabled until a Spectral ruleset is added. To re-enable: create
# .spectral.yaml (e.g. `extends: ["spectral:oas"]`) and uncomment below.
# lint: ## Lint OpenAPI spec with Spectral
# 	@echo "Linting OpenAPI spec..."
# 	spectral lint combined_api/openapi.yaml --ruleset .spectral.yaml

mock: ## Run Prism mock server
	prism mock combined_api/openapi.yaml -p 4010 --cors

merge: ## Merge service specs into combined_api/openapi.yaml
	@echo "Merging service specs into combined_api/openapi.yaml..."
	@if command -v npx >/dev/null 2>&1 && [ -f combined_api/openapi-merge.json ]; then \
		cd combined_api && npx --yes openapi-merge-cli --config openapi-merge.json && \
		echo "Fixing OpenAPI version to 3.1.1..." && \
		sed -i '' 's/^openapi: .*/openapi: 3.1.1/' openapi.yaml; \
	else \
		echo "Merge skipped: npx or openapi-merge.json not found"; \
	fi

up: ## Start all services with docker-compose
	docker compose up -d
	@echo ""
	@echo "Services are running:"
	@echo "  - Prism Mock Server: http://localhost:4010"
	@echo "  - Scalar Documentation: http://localhost:8010"

logs: ## Show docker-compose logs
	docker compose logs -f

down: ## Stop all docker-compose services
	docker compose down

clean: ## Clean up docker resources
	docker compose down -v