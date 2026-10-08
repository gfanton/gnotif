# Deploying the registry

These steps deploy a gnotif registry to onyx (`onyx-1`), for an operator who runs a gnotif server on their own registry. The repository's realms sit under the placeholder namespace `gno.land/r/dev`, and `make deploy-pkgs` writes copies under yours.

## Before you start

- **A namespace the deploying key owns.** Deploy the registry under the key's address, `gno.land/r/<address>`. The chain refuses a deploy under anyone else's namespace, and the [verified mark](how-it-works.md#what-the-verified-mark-means) relies on that. A registered name can change hands, and GovDAO controls the deploy gate ([Rules for a new registry version](how-it-works.md#rules-for-a-new-registry-version)).
- **gnokey and a key that holds test GNOT on onyx**, as in [Getting started](getting-started.md#4-try-it-on-onyx). The deploy command names the key `mykey`: use your key's name. Every deploy is public, with the address that signed it, so use a key made for this rather than a personal one.

## 1. Write the deploy copies

```sh
make deploy-pkgs NS=gno.land/r/<namespace>
```

This writes `.tools/deploy/gnotif/v0`, the demo's echo realm in `.tools/deploy/echo/v0` and the pingpong example in `.tools/deploy/pingpong/v0`, without their tests. It rewrites every `gno.land/r/dev` in them to your namespace: the module paths and the realms' imports of the registry. The demo's README [deploys echo](../demo/README.md#deploy-it-to-onyx) once the registry is live.

## 2. Check the copies for personal data

A deployed file is public and permanent. Read every file the deploy will send, and look for names, handles, email addresses, local paths and keys:

```sh
find .tools/deploy -type f
```

## 3. Lint the realms

Lint the realms before deploying them:

```sh
make gno-lint
```

It lints the repository's realms with gno v1.5.0, the release the [Makefile](../Makefile) pins as `GNO_REF`, for CI too. The README's [Develop](../README.md#develop) section installs it with the realms' dependencies. The deploy copies differ from the realms only by namespace.

To check that onyx still runs v1.5.0, open https://rpc.onyx.testnets.gno.land/status and read `build_version`. When onyx runs another release, move the pin to it and pass the realm tests with it before deploying.

## 4. Deploy the registry

Note the indexer's latest height first. gnotifd takes it as `-start-height`, since the registry emits nothing before its deploy:

```sh
curl -s -X POST https://indexer.onyx.testnets.gno.land/graphql/query \
  -H 'Content-Type: application/json' -d '{"query":"{ latestBlockHeight }"}'
```

Deploy the registry. gnokey asks for your passphrase:

```sh
gnokey maketx addpkg -pkgpath gno.land/r/<namespace>/gnotif/v0 -pkgdir .tools/deploy/gnotif/v0 \
  -gas-wanted 40000000 -gas-fee 80000ugnot -max-deposit 20000000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

The deploy uses about 20 million gas, and the chain holds about 1.9 GNOT as the registry's storage deposit. [Getting started](getting-started.md#4-try-it-on-onyx) explains the flags.

onyx checks a new realm before it goes live, which takes a few seconds. Wait until the registry's page opens on gnoweb, at `https://onyx.testnets.gno.land/r/<namespace>/gnotif/v0`. It lists no trigger until a realm declares one. A realm that does not compile never goes live. If the page never opens, lint again and check that the key can pay the storage deposit, then deploy again to the same path with the same key. The new deploy replaces the package that never went live.

## 5. Point gnotifd at the registry

Start gnotifd on a new database, with `-registry gno.land/r/<namespace>/gnotif/v0` and `-start-height` set to the height from step 4 ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)). Once a realm declares a verified trigger, `GET /v1/triggers?target=<realm path>` lists it.

## 6. Deploy a later version

Deploy a new registry version at `gno.land/r/<namespace>/gnotif/v<N+1>`, in the same namespace and with the [rules for a new registry version](how-it-works.md#rules-for-a-new-registry-version). gnotifd follows it with no change to its flags.

On onyx and mainnet the package stays parked until an approver enables it. `init` runs in the enable transaction. When you start a new database at this version, a `-start-height` at or before the height of the deploy transaction is safe.
