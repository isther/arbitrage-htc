include .env

run:
	@go mod tidy

	@export TG_TOKEN=${TG_TOKEN} && \
	export BINANCE_API=${BINANCE_API} && \
	export BINANCE_SECRET=${BINANCE_SECRET} && \
	go run main.go

docker-run:
	@docker-compose up -d --force-recreate --remove-orphans --build cmd

clear-images:
	@docker image prune -a 

clear: 
	@rm logs/*
	@rm imgs/*
