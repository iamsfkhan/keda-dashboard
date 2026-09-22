.PHONY: all dev backend frontend frontend-build test lint build docker helm-lint validate-rbac clean

all: test build

dev:
	@echo "Run 'make backend' and 'make frontend' in separate terminals"

backend:
	go run ./cmd/dashboard

frontend:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

test:
	go test ./cmd/... ./internal/...
	cd frontend && npm test

lint:
	gofmt -w cmd internal
	go vet ./...
	cd frontend && npm run lint

build: frontend-build
	go build -o bin/keda-dashboard ./cmd/dashboard

docker:
	docker build -t keda-dashboard:dev .

helm-lint:
	helm lint charts/keda-dashboard
	helm template test charts/keda-dashboard >/dev/null

validate-rbac:
	./scripts/validate-rbac.sh

clean:
	rm -rf bin frontend/dist
