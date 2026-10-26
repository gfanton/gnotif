# Deploying the registry

These steps deploy a gnotif registry to onyx (`onyx-1`), for an operator who runs a gnotif server on their own registry. The repository's packages sit under the placeholder namespace `dev`, in `gno.land/r/dev` and `gno.land/p/dev`, and `make deploy-pkgs` writes copies under yours.

## Before you start

- **A namespace the deploying key owns.** Deploy the registry under the key's address, `gno.land/r/<address>`, and its trigger package under `gno.land/p/<address>`. The chain refuses a deploy under anyone else's namespace, and the [verified mark](how-it-works.md#what-the-verified-mark-means) relies on that. A registered name can change hands, and GovDAO controls the deploy gate ([Rules for a new registry version](#rules-for-a-new-registry-version)).
- **gnokey and a key that holds test GNOT on onyx**, as in [Getting started](getting-started.md#4-try-it-on-onyx). The deploy commands name the key `mykey`: use your key's name. Every deploy is public, with the address that signed it, so use a key made for this rather than a personal one.

## 1. Write the deploy copies

```sh
make deploy-pkgs NS=gno.land/r/<namespace>
```

This writes the trigger package in `.tools/deploy/trigger/v0`, the admins package in `.tools/deploy/admins/v0`, the registry in `.tools/deploy/gnotif/v1`, the demo's echo realm in `.tools/deploy/echo/v1` and the pingpong example in `.tools/deploy/pingpong/v0`, without their tests. It rewrites every `gno.land/r/dev` in them to your namespace, and every `gno.land/p/dev` to the same namespace under `gno.land/p/`: the module paths and the imports of the registry and its packages. The demo's README [deploys echo](../demo/README.md#deploy-it-to-onyx) once the registry is live.

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

It lints the repository's realms with gno v1.6.0-rc.1, the release the [Makefile](../Makefile) pins as `GNO_REF`, and installs it on the first run. The deploy copies differ from the realms only by namespace.

To check that onyx still runs v1.6.0-rc.1, open https://rpc.onyx.testnets.gno.land/status and read `build_version`. When onyx runs another release, move the pin to it and pass the realm tests with it before deploying.

## 4. Deploy the registry

The registry imports its trigger package, which holds the `Trigger` struct, so deploy the package first. gnokey asks for your passphrase:

```sh
gnokey maketx addpkg -pkgpath gno.land/p/<namespace>/gnotif/trigger/v0 -pkgdir .tools/deploy/trigger/v0 \
  -gas-wanted 10000000 -gas-fee 20000ugnot -max-deposit 10000000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

onyx checks a new package before it goes live, which takes a few seconds. Wait until the package's page opens on gnoweb, at `https://onyx.testnets.gno.land/p/<namespace>/gnotif/trigger/v0`.

Deploy the admins package, which a dapp imports to let a list of addresses change its triggers ([Managing triggers](managing-triggers.md#let-admins-change-your-triggers)). It imports the trigger package:

```sh
gnokey maketx addpkg -pkgpath gno.land/p/<namespace>/gnotif/admins/v0 -pkgdir .tools/deploy/admins/v0 \
  -gas-wanted 10000000 -gas-fee 20000ugnot -max-deposit 10000000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

Wait until its page opens, at `https://onyx.testnets.gno.land/p/<namespace>/gnotif/admins/v0`.

Then note the indexer's latest height. gnotifd takes it as `-start-height`, since the registry emits nothing before its deploy:

```sh
curl -s -X POST https://indexer.onyx.testnets.gno.land/graphql/query \
  -H 'Content-Type: application/json' -d '{"query":"{ latestBlockHeight }"}'
```

Deploy the registry:

```sh
gnokey maketx addpkg -pkgpath gno.land/r/<namespace>/gnotif/v1 -pkgdir .tools/deploy/gnotif/v1 \
  -gas-wanted 60000000 -gas-fee 120000ugnot -max-deposit 20000000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

The package's deploy uses about 4 million gas and holds about 0.12 GNOT as its storage deposit. The registry's uses about 40 million gas and holds about 2.8 GNOT. [Getting started](getting-started.md#4-try-it-on-onyx) explains the flags.

Wait until the registry's page opens on gnoweb, at `https://onyx.testnets.gno.land/r/<namespace>/gnotif/v1`. It lists no trigger until a realm sets one. A realm that does not compile never goes live. If the page never opens, lint again and check that the key can pay the storage deposit, then deploy again to the same path with the same key. The new deploy replaces the package that never went live.

## 5. Point gnotifd at the registry

Start gnotifd on a new database, with `-registry gno.land/r/<namespace>/gnotif/v1` and `-start-height` set to the height from step 4 ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)). Once a realm sets a verified trigger, `GET /v1/triggers?target=<realm path>` lists it.

## 6. Deploy a later version

Deploy a new registry version at `gno.land/r/<namespace>/gnotif/v<N+1>`, in the same namespace and with the [rules for a new registry version](#rules-for-a-new-registry-version). gnotifd follows it with no change to its flags.

On onyx and mainnet the package stays parked until an approver enables it. `init` runs in the enable transaction. When you start a new database at this version, a `-start-height` at or before the height of the deploy transaction is safe.

### Rules for a new registry version

The registry is immutable, so a change ships as a new version at `…/gnotif/v<N+1>`. gnotifd follows every later version, so a new version must keep these rules:

1. **Path.** Deploy it at `…/gnotif/v<N+1>`, in the same namespace.
2. **Not private.** Never set `private = true`. The creator of a private package can redeploy it in place. `init` then runs again with fresh globals, the id counter restarts, and the version reissues its own ids over the stored triggers.
3. **Ids** take the form `v<N+1>-…`.
4. **Existing attributes are frozen.** Never remove, rename or change the meaning of a `TriggerDeclared` or `TriggerRemoved` attribute. gnotifd skips a `TriggerDeclared` that lacks one of the ten. A version may emit `TriggerDeclared` again for one of its own ids to change its title, body or link; it never changes that id's target, event, filter or param, and gnotifd skips a declaration that does.
5. **Attribute hygiene.** Never emit the same key twice, since gnotifd reads the first. Stay within the chain's caps for one event: 64 attribute pairs, and at most 4096 bytes for each type, key and value. v1 uses 12 pairs.
6. **New attributes** must be safe to ignore: presentation only, such as an icon.
7. **Narrowing changes go in a new event type.** Anything that narrows matching, such as an extra filter or an expiry, goes in a new event type, which older servers skip. A new event type may only add triggers. Removal stays `TriggerRemoved`, and so does anything else that stops notifications, a pause included: an older server skips an event type it does not know, and would keep sending.
8. **Emit from the realm package itself.** Call `chain.Emit` directly in the realm. An emit moved into a `/p/` helper carries the helper's path, and servers miss it.
9. **No other emitters.** Emit registry events only from the registry's own exported functions, and never call a function or interface value a caller supplies.
10. **`verified` means the target realm's authority stands behind the trigger.** In v0 the caller presented the realm's live `cur`. From v1 the realm's gnotif identity, `<target>#gnotif`, set it with the handle the registry gave the realm in a transaction the target's namespace owner signed and sent straight to the realm. `target` never contains `#`.

gnotifd trusts every later version under the prefix and takes `verified` at face value. That trust rests on the chain's deploy gate, which lets only a namespace's owner deploy under it. GovDAO controls that gate, and the operators of a hardfork are a trust root too. An address namespace, `gno.land/r/<address>/gnotif/v<N>`, removes only the risk of a name changing hands; the GovDAO and hardfork risks remain.
