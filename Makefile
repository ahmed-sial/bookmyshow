build:
	@go build -o bin/api ./cmd/api

run:
	@CONFIG_PATH=config.yaml ./bin/api

sprint:	build run