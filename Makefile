run:
	@go mod tidy
	@go run main.go

docker-run:
	@docker-compose up -d --force-recreate --remove-orphans --build cmd

clear-images:
	@docker image prune -a 

clear: 
	@rm logs/*
	@rm imgs/*
