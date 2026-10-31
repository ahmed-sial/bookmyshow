build:
	@go build -o bin/api ./cmd/api

run:
	@CONFIG_PATH=config.yaml ./bin/api

sprint:	build run

migrate-up:
	@go build -o bin/migrate ./cmd/migrate
	@./bin/migrate up

migrate-down:
	@go build -o bin/migrate ./cmd/migrate
	@./bin/migrate down