.PHONY: build
build:
	dune build

.PHONY: run
run:
	@test -n "$(FILE)" || (echo "Usage: make run FILE=examples/factorial.rex" && exit 1)
	dune exec rexlang -- $(FILE)

.PHONY: test
test:
	dune runtest

.PHONY: clean
clean:
	dune clean

.PHONY: repl
repl:
	dune exec rexlang

.PHONY: watch
watch:
	dune build --watch

.PHONY: fmt
fmt:
	dune fmt
