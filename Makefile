.PHONY: run test migrate-up migrate-down build

run:
	go run ./cmd/server/...

test:
	go test ./... -v -count=1

build:
	go build -o bin/server ./cmd/server/...

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1
