.PHONY: test check fast

# The command package's end-to-end tests exceed Go's default 10-minute
# per-package timeout; `go test ./...` with no flags fails on that alone.
test:
	go test -count=1 -timeout 40m ./...

# Everything except the slow command package.
fast:
	go test -count=1 ./internal/...

check:
	test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go vet ./...
	$(MAKE) fast
