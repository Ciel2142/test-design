.PHONY: test check fast test-opencode install-opencode

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

# The OpenCode adapter's unit tests (Node 22.7+).
test-opencode:
	node --test 'opencode/*.test.js'

# Link the OpenCode adapter into the global plugins directory. Never replaces
# an entry that points anywhere else.
install-opencode:
	@dir="$${XDG_CONFIG_HOME:-$$HOME/.config}/opencode/plugins"; \
	src="$$(pwd -P)/opencode/plugin.js"; dest="$$dir/test-design.js"; \
	mkdir -p "$$dir"; \
	if [ -L "$$dest" ] && [ "$$(readlink "$$dest")" = "$$src" ]; then \
	  echo "already linked: $$dest -> $$src"; \
	elif [ -e "$$dest" ] || [ -L "$$dest" ]; then \
	  echo "refusing to replace $$dest; remove it first" >&2; exit 1; \
	else \
	  ln -s "$$src" "$$dest" && echo "linked: $$dest -> $$src"; \
	fi
