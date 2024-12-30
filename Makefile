
.PHONY: run
run:
	docker-compose down --remove-orphans
	docker-compose up -d

.PHONY: stop
stop:
	docker-compose down

.PHONY: lint
lint:
	golangci-lint run -v ./...

.PHONY: build
build:
	mkdir -p build
	GOOS=linux GOARCH=amd64 go build -o build/broadcaster ./cmd/broadcaster/main.go

.PHONY: clean
clean:
	rm ./build/*

.PHONY: executable
executable:
	chmod 400 ./infra/private_keys/cloudtls.pem
