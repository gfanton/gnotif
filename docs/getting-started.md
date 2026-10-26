# Getting started

This guide adds browser notifications to a gno.land dapp in four steps. Your realm emits an event. A trigger in the gnotif registry says who that event notifies. Your page opts the browser in with the `gnotif` npm package. Then you try it on the onyx testnet, where the public server at gnotif.xyz sends the notifications.

You need:

- a page served over https or from `localhost`, in a browser with Web Push, such as Chrome;
- for step 4, gnokey, the gno.land command-line tool, and a key that holds test GNOT on onyx. [Interacting with Gno.land using gnokey](https://docs.gno.land/users/interact-with-gnokey/) covers installing gnokey, making a key and getting test GNOT from the faucet.

The example is a small realm with one function, `Notify`, that sends a message to an address. The demo at https://demo.gnotif.xyz is a complete dapp.

## 1. Emit an event

A realm emits an event with `chain.Emit`: an event type, then pairs of attribute names and values. `Notify` emits a `Message` event that says who the message is for and what it says:

```gno
// Notify sends msg to the browsers that opted in with the address to.
func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}
```

The chain marks every event with the path of the realm that emitted it, so no other realm can emit an event in your realm's name.

## 2. Set the trigger

A trigger tells gnotif which event to watch and what notification to send. Your realm sets it in the gnotif registry from its `init` function, which runs once, when the realm is deployed. Here is the whole realm:

```gno
package notify

import (
	"chain"

	"gno.land/r/nym-gfanton001/gnotif/v1"
)

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "message", gnotif.Trigger{
		Event: "Message", Param: "to",
		Title: "New message", Body: "{msg}", Link: "/",
	})
}

// Notify sends msg to the browsers that opted in with the address to.
func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}
```

`gno.land/r/nym-gfanton001/gnotif/v1` is the registry that gnotif.xyz reads. The trigger, named `message` in your realm, says:

- `Event`: watch the `Message` events of this realm;
- `Param`: a browser opts in with an address, and hears about the events whose `to` attribute is that address;
- `Title` and `Body`: the notification's title is "New message", and its text is the event's `msg`;
- `Link`: a click on the notification opens `/` on your dapp's site.

The first two lines of `init` open your realm's handle in the registry. `me` is your realm's gnotif identity, the only caller the registry takes your realm's triggers from, and `Open` gives your realm the handle that sets them, once. This realm keeps no reference to the handle, so its trigger never changes. [Managing triggers](managing-triggers.md#change-your-triggers-later) shows how to keep it.

> [!WARNING]
> With this realm, anyone can send a message to any address, and the text is whatever the caller sends. A dapp names an address in a watched event only once that address has acted, such as joining a game ([Who an event can notify](triggers.md#who-an-event-can-notify)).

## 3. Add the client to your page

Install the package:

```sh
npm install gnotif
```

The browser shows notifications through a [service worker](https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API), a script the browser runs for your site even when the page is closed. A browser only accepts one served by your own site, so copy the package's `sw.js` to the folder your site serves at its root, such as `public/` in Vite or Next.js:

```sh
cp node_modules/gnotif/dist/sw.js public/sw.js
```

Copy it again after each upgrade of the package.

Then, in your page's script:

```js
import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ network: "onyx" });
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
if (!message) throw new Error("gnotif.xyz has not read this realm's trigger yet");
const optins = [{ trigger: message.id, value: address }];

button.addEventListener("click", async () => {
  await gnotif.enable();
  await gnotif.setOptins(optins);
});
```

- `network: "onyx"` picks the public onyx server. It is the default.
- `gno.land/r/<you>/notify` is your realm's path, where `<you>` is the address you deploy it with.
- `triggers()` asks gnotif.xyz for your realm's triggers: here, the one from step 2. Ask on every page load, because a trigger's id can change ([Triggers](triggers.md#a-triggers-id-can-change)).
- `enable()` asks the browser for permission to show notifications, so call it from a click. `button` is a button on your page.
- `setOptins()` opts this browser in. `address` is the address of the person using the page, such as the one their wallet shows.

> [!TIP]
> An open page can also receive each notification as it happens, with no permission prompt: [Update an open page live](use-cases.md#update-an-open-page-live).

[The client's README](../js/README.md) documents each method, its errors, the `icon` option and the `server` option, which points the client at your own gnotifd.

## 4. Try it on onyx

The commands below name the key `mykey`: use your key's name.

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

`-gas-fee` is what the transaction costs, in ugnot: 60000ugnot is 0.06 GNOT. `-gas-wanted` caps the work it may do. The chain also holds a deposit for the storage of the realm and its trigger, about 0.4 GNOT, and `-max-deposit` caps it at 10 GNOT.

onyx checks a new realm before it goes live, which takes a few seconds. Wait until its page opens on gnoweb, at `https://onyx.testnets.gno.land/r/<you>/notify`. A realm that does not compile never goes live, so if the page never opens, compare your file with step 2.

Open your page, with your own address as `address`. If the page throws "gnotif.xyz has not read this realm's trigger yet", gnotif.xyz has not read your deploy yet: reload the page a few seconds later. Click the button, and allow notifications when the browser asks.

> [!TIP]
> On macOS, also allow notifications for the browser in System Settings.

Send yourself a message:

```sh
gnokey maketx call -pkgpath gno.land/r/<you>/notify -func Notify \
  -args <you> -args "Hello from onyx" \
  -gas-wanted 10000000 -gas-fee 20000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

A few seconds later, the browser shows "New message" with "Hello from onyx", even if your page is closed. A click on it opens your site at `/`.

## Next steps

- [Use cases](use-cases.md): triggers that tell a player it is their turn, the readers of a board about a new post, or everyone about a result, and a page that updates live.
- [Triggers](triggers.md): every field, the templates and how events match.
- [Managing triggers](managing-triggers.md): change a realm's triggers after its deploy, let admins change them, or stop them.
- [Running gnotifd](running-gnotifd.md): run your own gnotif server.
