.PHONY: build test fmt clean

build:
	go build -o rex ./cmd/rex/

test: build
	bash scripts/test-all.sh

fmt:
	gofmt -w .

clean:
	rm -f rex
