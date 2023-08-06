run:
	@go mod tidy
	@go run main.go

docker-cmd-run:
	@docker-compose up -d --force-recreate --build cmd

clear: 
	@rm logs/*
