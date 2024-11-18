
.PHONY: run-bsv
run-bsv:
	docker-compose down --remove-orphans
	docker-compose up -d

.PHONY: stop-bsv
stop-bsv:
	docker-compose down
