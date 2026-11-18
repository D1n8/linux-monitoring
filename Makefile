.PHONY: start stop test frontend build dev
start:
	./start.sh
stop:
	docker compose down
frontend:
	cd web && npm ci --no-audit --no-fund && npm run generate
	rm -rf internal/web/public
	mkdir -p internal/web/public
	cp -R web/.output/public/. internal/web/public/
build: frontend
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/goinmetrics ./cmd/goinmetrics
test:
	go test -race ./...
	go vet ./...
	cd web && npm run typecheck
dev:
	cd web && npm run dev
