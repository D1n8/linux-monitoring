.PHONY: start stop test frontend build dev image
start:
	./start.sh
stop:
	./stop.sh
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
	python3 -m unittest discover -s scripts/tests -v
	cd web && npm run typecheck && npm test
dev:
	cd web && npm run dev

image:
	./start.sh --build
