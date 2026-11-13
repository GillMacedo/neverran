.PHONY: test lint demo

test:
	test -z "$$(gofmt -l plugin tests examples)"
	go vet ./...
	cd examples/shop && go vet -tags integration ./...
	cd examples/shop/tools && go vet ./...
	go test -race ./...

lint:
	claude plugin validate ./plugin --strict
	claude plugin validate . --strict

# The CI-style run exits 1 because it has gaps on purpose.
demo:
	go run ./plugin/scripts --dir examples/shop examples/shop/runs/ci.json || test $$? -eq 1
	go run ./plugin/scripts --dir examples/shop examples/shop/runs/full.json examples/shop/runs/tools.json
