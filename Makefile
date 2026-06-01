.PHONY: all generate test clean

all: generate test

generate:
	go run ./cmd/gen/

test: generate
	go test ./...

clean:
	rm -f model/model.go
