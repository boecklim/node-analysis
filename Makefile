
.PHONY: run-bsv
run-bsv:
	docker compose -f docker-compose.bsv.yaml down --remove-orphans
	docker compose -f docker-compose.bsv.yaml up -d

.PHONY: stop-bsv
stop-bsv:
	docker compose -f docker-compose.bsv.yaml down


.PHONY: run-btc
run-btc:
	docker compose -f docker-compose.btc.yaml down --remove-orphans
	docker compose -f docker-compose.btc.yaml up

.PHONY: stop-btc
stop-btc:
	docker compose -f docker-compose.btc.yaml down
