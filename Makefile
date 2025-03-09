.PHONY: test

all:
	go clean && go build . && make test

lint:
	golangci-lint run

test:
	go test ./...

testcov:
	go test ./... -coverprofile=cover.out && go tool cover -html=cover.out
