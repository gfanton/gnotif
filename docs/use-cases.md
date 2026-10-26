# Use cases

A use case is one kind of notification: the event a realm emits, the trigger that watches it, and the opt-ins a page sends. Each realm sets its triggers from `init`, as in [Getting started](getting-started.md), which also shows how to deploy a realm and try it.

Pick the one closest to your dapp:

| To notify | The trigger |
|---|---|
| [one player, on their turn](#tell-a-player-its-their-turn) | `Param: "next"`, the player's address |
| [a board's readers, of a new post](#tell-the-readers-of-a-board) | `Param: "board"`, the board's name |
| [everyone, when a proposal passes](#announce-a-result-to-everyone) | no `Param`, and `Filter: "result=passed"` |
| [one bidder, of two kinds of news](#send-two-kinds-of-notification) | one trigger for each event |
| [the open page only, with no notification](#update-an-open-page-live) | any trigger, with `push: false` on the opt-in |

## Tell a player it's their turn

In a two-player game, each move tells the other player that it is their turn. The event names that player in its `next` attribute, and the trigger's `Param` is `next`. This is [pingpong](../demo/gno.land/r/pingpong/pingpong.gno), the example game in the demo folder:

```gno
func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "turn", gnotif.Trigger{
		Event: "TurnPlayed", Param: "next",
		Title: "Your turn", Body: "Game {game}, turn {turn}", Link: "/?game={game}",
	})
}

// Play takes the caller's turn in game and passes it to the other player.
func Play(cur realm, game string) {
	g := load(game)
	// ... check that the game started and the caller plays next, then pass the turn ...
	chain.Emit("TurnPlayed", "game", g.ID, "next", g.Next.String(), "turn", strconv.Itoa(g.Turn))
}
```

Each player's page opts in with that player's own address:

```js
const [turn] = await gnotif.triggers("gno.land/r/<you>/pingpong");
await gnotif.setOptins([{ trigger: turn.id, value: address }]);
```

The next player's browser shows "Your turn" with "Game 0000001, turn 3", and a click opens `/?game=0000001`.

A game starts only once the invited player accepts it, so `TurnPlayed` names only players who chose to play, and no stranger can make an address's browser show "Your turn" ([Who an event can notify](triggers.md#who-an-event-can-notify)).

## Tell the readers of a board

A forum tells the readers of a board about each new post. The event names the board in its `board` attribute, and a browser opts in once for each board it follows, with the board's name as the value:

```gno
h.Set(cross(me), "post", gnotif.Trigger{
	Event: "Posted", Param: "board",
	Title: "New post in {board}", Body: "{title}", Link: "/{board}",
})

// Post adds a post to board.
func Post(cur realm, board, title string) {
	// ... check that board exists and store the post ...
	chain.Emit("Posted", "board", board, "title", title)
}
```

```js
const [post] = await gnotif.triggers("gno.land/r/<you>/forum");
await gnotif.setOptins([
  { trigger: post.id, value: "general" },
  { trigger: post.id, value: "gno" },
]);
```

One trigger covers every board, and each browser hears only of the boards it follows. A board's name has to be unique in the realm, since two boards with one name share their readers. Anyone who can post can notify a board's readers, so the realm decides who may post.

## Announce a result to everyone

A DAO tells everyone who opted in when a proposal passes. The trigger has no `Param`, so every matching event notifies every browser that opted in to it. Its `Filter` keeps only the events whose `result` is `passed`:

```gno
h.Set(cross(me), "passed", gnotif.Trigger{
	Event: "ProposalClosed", Filter: "result=passed",
	Title: "Proposal {id} passed", Body: "{title}", Link: "/proposals/{id}",
})

// Close ends the vote on a proposal.
func Close(cur realm, id string) {
	p := load(id)
	// ... check that the vote is over, and count it ...
	chain.Emit("ProposalClosed", "id", p.ID, "result", p.Result, "title", p.Title)
}
```

A browser opts in to a trigger without a `Param` with `""` as the value:

```js
const [passed] = await gnotif.triggers("gno.land/r/<you>/dao");
await gnotif.setOptins([{ trigger: passed.id, value: "" }]);
```

A `ProposalClosed` whose `result` is `rejected` notifies no one. A `Filter` holds up to 8 `key=value` pairs, separated by commas, and an event has to carry every one of them ([Matching](triggers.md#matching)).

## Send two kinds of notification

An auction tells a bidder when someone outbids them, and tells the winner when it ends. Each kind is its own trigger, on its own event:

```gno
h.Set(cross(me), "outbid", gnotif.Trigger{
	Event: "Outbid", Param: "was",
	Title: "You were outbid", Body: "Lot {lot}, now {amount}", Link: "/lots/{lot}",
})
h.Set(cross(me), "won", gnotif.Trigger{
	Event: "Won", Param: "winner",
	Title: "You won lot {lot}", Body: "For {amount}", Link: "/lots/{lot}",
})
```

`Outbid` names the bidder who lost the lead in `was`, and `Won` names the winner in `winner`. Each event names only an address that placed a bid, so no caller can notify an address that never took part ([Who an event can notify](triggers.md#who-an-event-can-notify)).

A page opts in to both with the same address, and picks each trigger by its event ([A trigger's id can change](triggers.md#a-triggers-id-can-change)):

```js
const triggers = await gnotif.triggers("gno.land/r/<you>/auction");
const optins = ["Outbid", "Won"].map((event) => ({
  trigger: triggers.find((t) => t.event === event).id,
  value: address,
}));
await gnotif.setOptins(optins);
```

## Update an open page live

A game board redraws as soon as the other player moves, while its page is open. The page connects to the server and receives each notification live, with no permission prompt and no service worker. `push: false` keeps the opt-in off push, so the OS shows nothing for it:

```js
const [turn] = await gnotif.triggers("gno.land/r/<you>/pingpong");

gnotif.on("notification", () => redrawFromChain());
gnotif.on("reconnect", () => redrawFromChain());
await gnotif.setOptins([{ trigger: turn.id, value: address, push: false }]);
await gnotif.connect();
await redrawFromChain();
```

The page reads the board from the chain, in `redrawFromChain`, on every notification and after every reconnect. It registers its handlers, sets its opt-ins and connects before its first read, so a move that lands during that read is not lost. Without `push: false`, the same opt-in is also pushed once `enable()` has run, and the handler hears each notification once, however it arrives.

gnotif.xyz serves live updates, and a gnotifd of your own serves them once you set `-stream-max` ([Running gnotifd](running-gnotifd.md#flags)). [Notifications on the page](../js/README.md#notifications-on-the-page) lists what the handler receives and how the connection fails.
