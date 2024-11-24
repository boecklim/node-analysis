
.PHONY: run-bsv
run-bsv:
	docker-compose down --remove-orphans
	docker-compose up -d

.PHONY: stop-bsv
stop-bsv:
	docker-compose down

.PHONY: lint
lint:
	golangci-lint run -v ./...

.PHONY: build
build:
	mkdir -p build
	GOOS=linux GOARCH=amd64 go build -o build/listener ./cmd/listener/main.go
	GOOS=linux GOARCH=amd64 go build -o build/broadcaster ./cmd/broadcaster/main.go
