
.PHONY: run-bsv
run-bsv:
	docker-compose down -v --remove-orphans
	docker-compose up node1 node2

.PHONY: run-btc
run-btc:
	docker-compose -f ./docker-compose.btc.yaml down -v --remove-orphans
	docker-compose -f ./docker-compose.btc.yaml up node1 node2

.PHONY: run-tests-btc
run-tests-btc:
	docker-compose -f ./docker-compose.btc.yaml down -v --remove-orphans
	docker-compose -f ./docker-compose.btc.yaml up --abort-on-container-exit --build broadcaster1 broadcaster2
	docker-compose -f ./docker-compose.btc.yaml down

.PHONY: run-tests-bsv
run-tests-bsv:
	docker-compose down -v --remove-orphans
	docker-compose up --abort-on-container-exit --build broadcaster1 broadcaster2
	docker-compose down

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
