.PHONY: build test fmt clean

build:
	go build -o rex ./cmd/rex/

test: build
	go test ./...
	./rex --test internal/stdlib/rexfiles/*.rex examples/*.rex

fmt:
	gofmt -w .

clean:
	rm -f rex
