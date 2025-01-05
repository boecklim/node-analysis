
.PHONY: run-bsv-nodes
run-bsv-nodes:
	docker compose -f ./docker-compose.bsv.yaml down -v --remove-orphans
	docker compose -f ./docker-compose.bsv.yaml up node1 node2

.PHONY: run-btc-nodes
run-btc-nodes:
	docker compose -f ./docker-compose.btc.yaml down -v --remove-orphans
	docker compose -f ./docker-compose.btc.yaml up node1 node2

.PHONY: run-btc-nodes-with-broadcaster
run-btc-nodes-with-broadcaster:
	docker compose -f ./docker-compose.btc.yaml down -v --remove-orphans
	docker compose -f ./docker-compose.btc.yaml up --build broadcaster1 broadcaster2
	docker compose -f ./docker-compose.btc.yaml down

.PHONY: run-bsv-nodes-with-broadcaster
run-bsv-nodes-with-broadcaster:
	docker compose -f ./docker-compose.bsv.yaml down -v --remove-orphans
	docker compose -f ./docker-compose.bsv.yaml up --build broadcaster1 broadcaster2 broadcaster3 broadcaster4 broadcaster5
	docker compose -f ./docker-compose.bsv.yaml down

.PHONY: stop
stop:
	docker-compose down -v

.PHONY: lint
lint:
	golangci-lint run -v ./...

.PHONY: build
build:
	mkdir -p build
	GOOS=linux GOARCH=amd64 go build -o build/broadcaster ./cmd/broadcaster/main.go

.PHONY: build-docker
build-docker:
	docker build . -t test-broadcaster

.PHONY: clean
clean:
	rm ./build/*
