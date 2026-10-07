SHELL := /bin/sh
GNO_STORE ?= $(HOME)/.cache/gno-toolchains/onyx
GNO ?= $(GNO_STORE)/gno
GNOROOT_DIR = $(shell go env GOMODCACHE)/github.com/gnolang/gno@$(shell go version -m $(GNO) | awk '$$1 == "mod" {print $$3}')
GNO_ENV = GNOROOT=$(GNOROOT_DIR) GNOHOME=$(GNO_STORE)/gnohome
ONYX_RPC := https://rpc.onyx.testnets.gno.land:443
GNO_TEST_FLAGS ?=
TOOLS := .tools
GNO_CHECKOUT ?= https://github.com/gnolang/gno
GNO_REF := v1.5.0
TX_INDEXER_VERSION := v1.3.0
NS ?=
DEMO_JS := demo/gnotif.js demo/sw.js

.PHONY: all test gno-test gno-lint gno-deps go-test js-test tools e2e demo site site-serve deploy-pkgs clean help

all: test ## Run every test (default)

test: gno-test go-test js-test ## Run every test

gno-test: ## Run the realm tests
	$(GNO_ENV) $(GNO) test $(GNO_TEST_FLAGS) ./gno/... ./demo/...

gno-lint: ## Lint the realms
	$(GNO_ENV) $(GNO) lint ./gno/... ./demo/...

gno-deps: ## Fetch the realms' on-chain dependencies from onyx, once
	$(GNO_ENV) $(GNO) mod download -remote-overrides gno.land=$(ONYX_RPC)

go-test: ## Run go vet and the Go tests
	go vet ./...
	go test ./...

js-test: ## Run the browser script tests
	node --test 'js/test/*.test.mjs' 'demo/*.test.mjs'

# Marks .tools as a separate workspace, so the root one skips the gno checkout.
$(TOOLS)/gnowork.toml:
	mkdir -p $(@D)
	touch $@

$(TOOLS)/gno-src: | $(TOOLS)/gnowork.toml
	git clone --depth 1 --branch $(GNO_REF) $(GNO_CHECKOUT) $@

$(TOOLS)/gnodev: | $(TOOLS)/gno-src $(TOOLS)/gnowork.toml
	cd $(TOOLS)/gno-src/contribs/gnodev && go build -o $(abspath $@) .

$(TOOLS)/gnokey: | $(TOOLS)/gno-src $(TOOLS)/gnowork.toml
	cd $(TOOLS)/gno-src && go build -o $(abspath $@) ./gno.land/cmd/gnokey

$(TOOLS)/tx-indexer: | $(TOOLS)/gnowork.toml
	GOBIN=$(abspath $(TOOLS)) go install github.com/gnolang/tx-indexer/cmd@$(TX_INDEXER_VERSION)
	mv $(TOOLS)/cmd $@

tools: $(TOOLS)/gnodev $(TOOLS)/gnokey $(TOOLS)/tx-indexer ## Build gnodev, gnokey and tx-indexer for the end-to-end test

e2e: tools ## Run the local end-to-end test
	GNOTIF_TOOLS=$(abspath $(TOOLS)) GNOTIF_ROOT=$(abspath .) go test -tags e2e -count=1 -v ./e2e/

$(DEMO_JS): demo/%.js: js/src/%.js
	cp $< $@

demo: $(DEMO_JS) ## Serve the demo dapp on http://localhost:3000
	python3 -m http.server 3000 --bind 127.0.0.1 --directory demo

site: ## Build the website into site/dist
	rm -rf site/dist
	go run ./site -out site/dist

site-serve: site ## Build and serve the website on http://localhost:3001
	python3 -m http.server 3001 --bind 127.0.0.1 --directory site/dist

deploy-pkgs: ## Copy the realms to .tools/deploy under NS=gno.land/r/<namespace>, without tests
	test -n "$(NS)" || { echo "deploy-pkgs: set NS=gno.land/r/<namespace>" >&2; exit 1; }
	rm -rf $(TOOLS)/deploy
	mkdir -p $(TOOLS)/deploy/gnotif/v0 $(TOOLS)/deploy/echo/v0 $(TOOLS)/deploy/pingpong/v0
	cp -R gno/r/gnotif/v0/. $(TOOLS)/deploy/gnotif/v0/
	cp -R demo/gno.land/r/echo/v0/. $(TOOLS)/deploy/echo/v0/
	cp -R demo/gno.land/r/pingpong/v0/. $(TOOLS)/deploy/pingpong/v0/
	find $(TOOLS)/deploy \( -name '*_test.gno' -o -name filetests \) -prune -exec rm -rf {} +
	find $(TOOLS)/deploy -type f | while read -r f; do \
		sed 's|gno.land/r/dev|$(NS)|g' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f" || exit 1; \
	done
	touch $(TOOLS)/deploy/gnowork.toml

clean: ## Remove built tools, deploy copies, the website build and the demo's copied scripts
	rm -rf $(TOOLS) site/dist
	rm -f $(DEMO_JS)

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sed 's/:.*## / : /'
