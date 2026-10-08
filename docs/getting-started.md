# Getting started

This guide adds browser notifications to a gno.land dapp in four steps. Your realm emits an event. A trigger in the gnotif registry says who that event notifies. Your page opts the browser in with the `gnotif` npm package. Then you try it on the onyx testnet, where the public onyx server at `https://gnotif.xyz/onyx` sends the notifications.

The example is a small realm with one function, `Notify`, that sends a message to an address. For a complete dapp, see the demo at https://demo.gnotif.xyz.

## 1. Emit an event

A realm emits an event with `chain.Emit`: an event type, then pairs of attribute names and values. `Notify` emits a `Message` event that says who the message is for and what it says:

```go
// Notify sends msg to the browsers that opted in with the address to.
func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}
```

The chain marks every event with the path of the realm that emitted it, so no other realm can emit an event in your realm's name.

## 2. Declare the trigger

A trigger tells gnotif which event to watch and what notification to send. Your realm declares it in the gnotif registry, from its `init` function. `init` runs once, when the realm is deployed, so the deploy declares the trigger, and nobody can run `init` again.

Here is the whole realm:

```go
package notify

import (
	"chain"

	"gno.land/r/nym-gfanton001/gnotif/v0"
)

func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "Message", "", "to", "New message", "{msg}", "/")
}

// Notify sends msg to the browsers that opted in with the address to.
func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}
```

`gno.land/r/nym-gfanton001/gnotif/v0` is the registry that gnotif.xyz reads. The `Declare` call says:

- watch the `Message` events of this realm, whose path is `cur.PkgPath()`;
- a browser opts in with an address, and hears about the events whose `to` attribute is that address;
- the notification's title is "New message", and its text is the event's `msg`;
- a click on the notification opens `/` on your dapp's site.

The empty string after `"Message"` is a filter, which this trigger does not use. To notify every browser that opted in, whatever its address, leave the param empty: `""` in place of `"to"`, and opt in with `value: ""`.

`cross(cur)` makes your realm the caller that the registry sees. The registry accepts a trigger only when its caller is the realm the trigger watches.

With this realm, anyone can send a message to any address, and the text is whatever the caller sends.

[Triggers](triggers.md) covers every field, the templates and how events match.

## 3. Add the client to your page

Install the package:

```sh
npm install gnotif
```

The browser shows notifications through a [service worker](https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API), a script the browser runs for your site even when the page is closed. A browser only accepts one served by your own site, so copy the package's `sw.js` to the folder your site serves at its root, such as `public/` in Vite or Next.js:

```sh
cp node_modules/gnotif/src/sw.js public/sw.js
```

Copy it again after each upgrade of the package.

Then, in your page's script:

```js
import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ network: "onyx" });
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
if (!message) throw new Error("gnotif.xyz has not read this realm's trigger yet");

button.addEventListener("click", async () => {
  await gnotif.enable();
  await gnotif.setOptins([{ trigger: message.id, value: address }]);
});
```

- `network: "onyx"` picks the public onyx server. It is the default, and `server` points the client at your own gnotifd instead ([the client's README](../js/README.md)).
- `gno.land/r/<you>/notify` is your realm's path, where `<you>` is the address you deploy it with.
- `triggers()` asks gnotif.xyz for your realm's triggers. This realm has one, the trigger from step 2. Ask on every page load, because a trigger's id can change ([Triggers](triggers.md#a-triggers-id-can-change)).
- `enable()` asks the browser for permission to show notifications, so call it from a click. `button` is a button on your page.
- `setOptins()` opts this browser in. `address` is the address of the person using the page, such as the one their wallet shows.

[The client's README](../js/README.md) documents each method and its errors.

## 4. Try it on onyx

This step needs gnokey, the gno.land command-line tool, and a key that holds test GNOT on onyx. If you don't have them yet, [Interacting with Gno.land using gnokey](https://docs.gno.land/users/interact-with-gnokey/) covers installing gnokey, making a key and getting test GNOT from the faucet. The commands below name the key `mykey`: use your key's name.

Make a folder named `notify`. Save the realm from step 2 in it as `notify.gno`, and add a file named `gnomod.toml`:

```toml
module = "gno.land/r/<you>/notify"
gno = "0.9"
```

Here and in the commands below, replace `<you>` with your key's address, which starts with `g1`.

From inside the `notify` folder, deploy the realm. gnokey asks for your passphrase:

```sh
gnokey maketx addpkg -pkgpath gno.land/r/<you>/notify -pkgdir . \
  -gas-wanted 30000000 -gas-fee 60000ugnot -max-deposit 10000000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

`-gas-fee` is what the transaction costs, in ugnot: 60000ugnot is 0.06 GNOT. `-gas-wanted` caps the work it may do. The chain also holds a deposit for the realm's storage, about 1 GNOT, and `-max-deposit` caps it at 10 GNOT.

onyx checks a new realm before it goes live, which takes a few seconds. Wait until its page opens on gnoweb, at `https://onyx.testnets.gno.land/r/<you>/notify`. A realm that does not compile never goes live, so if the page never opens, compare your file with step 2.

Serve your page over https or from `localhost`, and use a browser with Web Push, such as Chrome. On macOS, also allow notifications for the browser in System Settings.

Open your page, with your own address as `address`. If the page throws "gnotif.xyz has not read this realm's trigger yet", gnotif.xyz has not read your deploy yet: reload the page a few seconds later. Click the button, and allow notifications when the browser asks.

Send yourself a message:

```sh
gnokey maketx call -pkgpath gno.land/r/<you>/notify -func Notify \
  -args <you> -args "Hello from onyx" \
  -gas-wanted 10000000 -gas-fee 20000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

A few seconds later, the browser shows "New message" with "Hello from onyx", even if your page is closed. A click on it opens your site at `/`.

To run your own gnotif server, see [Running gnotifd](running-gnotifd.md).
