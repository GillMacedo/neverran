# Examples

`shop/` is a made-up Go module, `example.com/shop`, with four common gaps:

- `tools/` has its own `go.mod`, so `go test ./...` at the root doesn't test it.
- `store/store_integration_test.go` has `//go:build integration`.
- `search/search_test.go` has a `TestMain` that exits 0 when `SHOP_SEARCH_FIXTURES` isn't set.
- `payments/payments_test.go` has a test that calls `t.Skip` when `SHOP_PAYMENTS_SANDBOX` isn't set.

`shop/runs/` holds real `go test -json` output, made with Go 1.27.3 on linux/arm64:

    cd examples/shop
    go test -count=1 -json ./... > runs/ci.json
    SHOP_PAYMENTS_SANDBOX=1 SHOP_SEARCH_FIXTURES=testdata/products.txt go test -count=1 -json -tags integration ./... > runs/full.json
    cd tools && go test -count=1 -json ./... > ../runs/tools.json

`ci.json` is the usual CI step and has every gap. `full.json` and `tools.json` together run all 10 tests. See the main README for neverran's output on each.
