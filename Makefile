SHELL := /bin/sh
GNO_REF := v1.5.0
CACHE := $(CURDIR)/.cache
GNO_STORE := $(CACHE)/gno-$(GNO_REF)
GNO := $(GNO_STORE)/gno
GNO_DEPS := $(GNO_STORE)/deps.stamp
GNO_MODS := $(wildcard gno/r/*/*/gnomod.toml demo/gno.land/r/*/*/gnomod.toml)
GNOROOT_DIR := $(shell go env GOMODCACHE)/github.com/gnolang/gno@$(GNO_REF)
GNO_ENV = GNOROOT=$(GNOROOT_DIR) GNOHOME=$(GNO_STORE)/gnohome
ONYX_RPC := https://rpc.onyx.testnets.gno.land:443
GNO_TEST_FLAGS ?=
TOOLS := .tools
GNO_CHECKOUT ?= https://github.com/gnolang/gno
TX_INDEXER_VERSION := v1.3.0
NS ?=
PNPM_STAMP := node_modules/.modules.yaml

.PHONY: all test gno-test gno-lint gno-deps go-test js-test tools e2e conformance demo site site-serve deploy-pkgs clean fclean help

all: test ## Run every test (default)

test: gno-test go-test js-test ## Run every test

# Installing a new GNO_REF removes the toolchains of the other releases.
$(GNO):
	rm -rf $(filter-out $(GNO_STORE),$(wildcard $(CACHE)/gno-*))
	GOBIN=$(GNO_STORE) go install github.com/gnolang/gno/gnovm/cmd/gno@$(GNO_REF)

$(GNO_DEPS): $(GNO) $(GNO_MODS)
	$(GNO_ENV) $(GNO) mod download -remote-overrides gno.land=$(ONYX_RPC)
	touch $@

gno-test: $(GNO_DEPS) ## Run the realm tests
	$(GNO_ENV) $(GNO) test $(GNO_TEST_FLAGS) ./gno/... ./demo/...

gno-lint: $(GNO_DEPS) ## Lint the realms
	$(GNO_ENV) $(GNO) lint ./gno/... ./demo/...

gno-deps: $(GNO_DEPS) ## Install the pinned gno and fetch the realms' on-chain dependencies from onyx

go-test: ## Run go vet and the Go tests
	go vet ./...
	go test ./...

$(PNPM_STAMP): pnpm-lock.yaml pnpm-workspace.yaml package.json js/package.json demo/package.json
	pnpm install --frozen-lockfile
	touch $@

js-test: $(PNPM_STAMP) ## Run the browser script tests
	pnpm test

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

conformance: ## Check the live endpoint of GNOTIF_SERVER, opting into a trigger of GNOTIF_TARGET
	test -n "$(GNOTIF_SERVER)" && test -n "$(GNOTIF_TARGET)" || { echo "conformance: set GNOTIF_SERVER=<gnotif server URL> GNOTIF_TARGET=<realm path>" >&2; exit 1; }
	GNOTIF_SERVER=$(GNOTIF_SERVER) GNOTIF_TARGET=$(GNOTIF_TARGET) node --test e2e/conformance/stream.test.mjs

demo: $(PNPM_STAMP) ## Build the demo dapp into demo/dist and serve it on http://localhost:3000
	pnpm build
	pnpm --filter gnotif-demo serve

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

clean: ## Remove built tools, deploy copies, the website build, the package build and the demo build
	rm -rf $(TOOLS) site/dist js/dist demo/dist

fclean: clean ## Also remove the gno toolchain, the realms' dependencies in .cache and the installed JavaScript packages
	rm -rf $(CACHE) node_modules js/node_modules demo/node_modules

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sed 's/:.*## / : /'
