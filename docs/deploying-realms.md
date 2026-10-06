# Deploying the registry

These steps deploy a gnotif registry to onyx (`onyx-1`), for an operator who runs a gnotif server on their own registry. The steps use gnomcp, a Model Context Protocol (MCP) server that gives an AI client tools to read and write a gno.land chain. The repository's realms sit under the placeholder namespace `gno.land/r/dev`, and `make deploy-pkgs` writes copies under yours. [The demo's README](../demo/README.md#deploy-it-to-onyx) deploys pingpong on top of the registry.

## Before you start

- **A namespace the deploying key owns.** On onyx a key deploys under its own address, `gno.land/r/<address>`, or under a name it registered. The chain refuses a deploy under anyone else's namespace, and the [verified mark](how-it-works.md#what-the-verified-mark-means) relies on that.
- **gnomcp connected to onyx.** `gno_status` reports the chain id, `onyx-1`.
- **gnomcp's agent key, never a personal key.** The agent key is the key gnomcp holds and signs with: `gno_key_address` shows it, and `gno_key_generate` makes one on a testnet. Every deploy and call is public, with the address that signed it.
- **Funds for the storage deposits.** `gno_faucet_fund` funds the agent key on onyx.

## 1. Write the deploy copies

```sh
make deploy-pkgs NS=gno.land/r/<namespace>
```

This writes `.tools/deploy/gnotif/v0`, and pingpong's copy for the demo in `.tools/deploy/pingpong/v0`, without their tests. It rewrites every `gno.land/r/dev` in them to your namespace: both module paths and pingpong's import of the registry. It also writes `.tools/deploy/gnowork.toml`, which makes the copies one workspace for the lint in step 3. The marker sits beside the packages, outside them, and is not deployed.

## 2. Check the copies for personal data

A deployed file is public and permanent. Read every file the deploy will send, and look for names, handles, email addresses, local paths and keys:

```sh
find .tools/deploy -type f
```

## 3. Lint the copies with the chain's toolchain

onyx parks every new package until an automatic approver type-checks it and enables it. A package that fails the check stays parked, and the chain reports it exactly like one still waiting. A dry run does not type-check. Lint the copies with the gno release onyx runs before deploying them.

The Makefile's toolchain is that release: CI pins it as `GNO_VERSION` in [ci.yml](../.github/workflows/ci.yml), and the README's [Develop](../README.md#develop) section installs it with the realms' dependencies. Check that onyx still runs it:

```sh
curl -s https://rpc.onyx.testnets.gno.land/status | grep build_version
```

When onyx reports another release, move the pin to it and pass the realm tests with it before deploying.

Then lint the copies with the Makefile's toolchain. The workspace marker makes pingpong's import of the registry resolve to the copy beside it. `store` is the Makefile's `GNO_STORE`:

```sh
store=$HOME/.cache/gno-toolchains/onyx
gnoroot="$(go env GOMODCACHE)/github.com/gnolang/gno@$(go version -m "$store/gno" | awk '$1 == "mod" {print $3}')"
(cd .tools/deploy && GNOROOT="$gnoroot" GNOHOME="$store/gnohome" "$store/gno" lint ./...)
```

`GNOROOT` points the binary at the standard library of its own release.

## 4. Deploy the registry

Note the indexer's latest height first. gnotifd takes it as `-start-height`, since the registry emits nothing before its deploy:

```sh
curl -s -X POST https://indexer.onyx.testnets.gno.land/graphql/query \
  -H 'Content-Type: application/json' -d '{"query":"{ latestBlockHeight }"}'
```

Deploy `.tools/deploy/gnotif/v0` at `gno.land/r/<namespace>/gnotif/v0` with `gno_addpkg`. It waits for the approver and reports `package_status`:

| `package_status` | Meaning | Next |
|---|---|---|
| `live` | enabled and callable | go on |
| `inert` | parked, not callable | when it is still parked a minute later: lint again, check that the key can pay the storage deposit, then deploy to the same path with the same key, which replaces the parked package |
| `redeploy_parked` | a new version is parked; the previous one keeps serving | as for `inert` |
| `unknown` | the chain gave no usable answer | read the path with `gno_read` before going on |

The registry's page renders on gnoweb, at `https://onyx.testnets.gno.land/r/<namespace>/gnotif/v0`, or with `gno_render` on `gno.land/r/<namespace>/gnotif/v0`. It lists no trigger until a realm declares one.

## 5. Point gnotifd at the registry

Start gnotifd on a new database, with `-registry gno.land/r/<namespace>/gnotif/v0` and `-start-height` set to the height from step 4 ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)). Once a realm declares a verified trigger, `GET /v1/triggers?target=<realm path>` lists it.
