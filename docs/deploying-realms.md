# Deploying the realms

The realms deploy to onyx (`onyx-1`) through gnomcp, in a fixed order: the registry, then pingpong, then a call to pingpong's `DeclareTriggers`. The repository's realms sit under the placeholder namespace `gno.land/r/dev`, and `make deploy-pkgs` writes copies under yours.

## Before you start

- **A namespace the deploying key owns.** On onyx a key deploys under its own address, `gno.land/r/<address>`, or under a name it registered. The chain refuses a deploy under anyone else's namespace, and the [verified mark](how-it-works.md#what-the-verified-mark-means) relies on that.
- **gnomcp connected to onyx.** `gno_status` reports the chain id, `onyx-1`.
- **gnomcp's agent key, never a personal key.** Every deploy and call is public, with the address that signed it.
- **Funds for the storage deposits.** `gno_faucet_fund` funds the agent key on onyx.

## 1. Write the deploy copies

```sh
make deploy-pkgs NS=gno.land/r/<namespace>
```

This writes `.tools/deploy/gnotif/v0` and `.tools/deploy/pingpong/v0` without their tests, and rewrites every `gno.land/r/dev` in them to your namespace: both module paths and pingpong's import of the registry. pingpong declares its trigger with `cur.PkgPath()`, so the trigger follows the new path.

## 2. Check the copies for personal data

A deployed file is public and permanent. Read every file the deploy will send, and look for names, handles, email addresses, local paths and keys:

```sh
find .tools/deploy -type f
```

## 3. Lint the copies with the chain's toolchain

onyx parks every new package until an automatic approver type-checks it and enables it. A package that fails the check stays parked, and the chain reports it exactly like one still waiting. A dry run does not type-check either. Lint the copies with the gno release onyx runs before deploying them.

Read the release from the node:

```sh
curl -s https://rpc.onyx.testnets.gno.land/status | grep build_version
```

Install that release, here `v1.5.0`, into a toolchain store of its own, apart from the one `make test` reads, and fetch the realms' dependencies from onyx into it:

```sh
store=$HOME/.cache/gno-toolchains/onyx
GOBIN=$store go install github.com/gnolang/gno/gnovm/cmd/gno@v1.5.0
make gno-deps GNO_STORE=$store
```

Then lint the copies as one workspace, so that pingpong's import of the registry resolves to the copy beside it:

```sh
gnoroot="$(go env GOMODCACHE)/github.com/gnolang/gno@$(go version -m "$store/gno" | awk '$1 == "mod" {print $3}')"
touch .tools/deploy/gnowork.toml
(cd .tools/deploy && GNOROOT="$gnoroot" GNOHOME="$store/gnohome" "$store/gno" lint ./...)
```

`GNOROOT` points the binary at the standard library of its own release. `make deploy-pkgs` starts from an empty `.tools/deploy`, so touch `gnowork.toml` again after rerunning it. The marker sits beside the packages, outside them, and is not deployed.

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

## 5. Deploy pingpong

pingpong imports the registry, so deploy it only once the registry is `live`. Deploy `.tools/deploy/pingpong/v0` at `gno.land/r/<namespace>/pingpong/v0` with `gno_addpkg`, and wait for `live`.

## 6. Declare the trigger

Call pingpong's `DeclareTriggers` with `gno_call`. It takes no argument and works once; a second call panics with `triggers already declared`.

After the call, the registry's page lists the trigger with "✓ verified": `gno_render` on `gno.land/r/<namespace>/gnotif/v0`, or https://onyx.testnets.gno.land/r/<namespace>/gnotif/v0 on gnoweb.

## 7. Point gnotifd and the demo at the deploy

Start gnotifd with `-registry gno.land/r/<namespace>/gnotif/v0` and the height from step 4 ([Running gnotifd](running-gnotifd.md#start-gnotifd)). Once it has read the deploy, `GET /v1/triggers` lists the trigger with `"verified": true`.

For the demo, set `demo/config.js` to the deploy: `server` to the gnotifd URL, `pingpong` to `gno.land/r/<namespace>/pingpong/v0`, and `gnoweb` to `https://onyx.testnets.gno.land`.
