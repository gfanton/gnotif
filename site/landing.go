package main

import "html/template"

// step is one of the three integration steps shown on the landing page.
// Text is the site's own copy and may carry inline code.
type step struct {
	Label    string
	Title    string
	Text     template.HTML
	Lang     string
	Code     string
	DocsHref string
}

var steps = [3]step{
	{
		Label: "Emit",
		Title: "Emit an event from your realm",
		Text:  "A realm with one function, <code>Notify</code>, emits a message and the address it is for.",
		Lang:  "go",
		Code: `func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}`,
		DocsHref: "/docs/getting-started/#1-emit-an-event",
	},
	{
		Label: "Declare",
		Title: "Declare a trigger in the registry",
		Text:  "The realm declares the trigger itself, in <code>init</code>, so the deploy declares it and the registry marks it verified.",
		Lang:  "go",
		Code: `import "` + registryPath + `"

func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "Message", "", "to",
		"New message", "{msg}", "/")
}`,
		DocsHref: "/docs/getting-started/#2-declare-the-trigger",
	},
	{
		Label: "Subscribe",
		Title: "Subscribe the browser from your page",
		Text:  "Copy <code>node_modules/gnotif/src/sw.js</code> to the folder your site serves at its root. From a click, call <code>enable()</code>, then opt the browser in with the user's address.",
		Lang:  "js",
		Code: `import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ server: "` + serverURL + `" });
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
if (!message) throw new Error("gnotif.xyz has not read this realm's trigger yet");

button.addEventListener("click", async () => {
  await gnotif.enable();
  await gnotif.setOptins([{ trigger: message.id, value: address }]);
});`,
		DocsHref: "/docs/getting-started/#3-add-the-client-to-your-page",
	},
}

type renderedStep struct {
	Step step
	Code template.HTML
}

// scene is one example on the hero's signal line: the event a realm emits
// on the left, the notification it becomes on the right.
type scene struct {
	Label string
	Event string
	Attrs string
	Title string
	Body  string
	App   string
	Who   string
}

var scenes = []scene{
	{Label: "pingpong · TurnPlayed", Event: "TurnPlayed", Attrs: "game=0000042 next=g1k7…x2q turn=7",
		Title: "Your turn", Body: "Game 0000042, turn 7", App: "pingpong", Who: "the player's browser"},
	{Label: "gnochat · Mentioned", Event: "Mentioned", Attrs: "channel=general by=g1zm…9ra who=g1k7…x2q",
		Title: "g1zm…9ra mentioned you", Body: "in #general", App: "gnochat", Who: "the member's browser"},
	{Label: "govdao · ProposalClosed", Event: "ProposalClosed", Attrs: "id=0000012 result=passed",
		Title: "Proposal 12 passed", Body: "Treasury budget, Q4", App: "govdao", Who: "every voter's browser"},
	{Label: "auctions · Outbid", Event: "Outbid", Attrs: "lot=0000309 by=g1p0…7cd was=g1k7…x2q",
		Title: "You were outbid", Body: "Lot 309, now 1 250 GNOT", App: "auctions", Who: "the bidder's browser"},
	{Label: "vesting · UnlockReached", Event: "UnlockReached", Attrs: "height=1300000 account=g1k7…x2q",
		Title: "Block 1 300 000 reached", Body: "Your tokens are unlocked", App: "vesting", Who: "the holder's browser"},
}

// flowNode is one box of the "How it works" path. Dashed nodes belong to
// the dapp, solid ones to gnotif and the chain's infrastructure.
type flowNode struct {
	Name   string
	Note   string
	Dashed bool
}

var flow = []flowNode{
	{Name: "dapp realm", Note: "emits events, declares its trigger", Dashed: true},
	{Name: "gnotif registry", Note: "holds the triggers, on chain"},
	{Name: "tx-indexer", Note: "reads the chain, serves GraphQL"},
	{Name: "gnotifd", Note: "matches events, sends the pushes"},
	{Name: "push service", Note: "Web Push, run by the browser's vendor"},
	{Name: "sw.js", Note: "shows the notification, on the dapp's origin", Dashed: true},
}

type landingView struct {
	Site   siteView
	Scenes []scene
	Steps  []renderedStep
	Flow   []flowNode
}

func newLandingView() (landingView, error) {
	v := landingView{Site: site, Scenes: scenes, Flow: flow}
	for _, s := range steps {
		code, err := highlight(s.Lang, s.Code)
		if err != nil {
			return landingView{}, err
		}
		v.Steps = append(v.Steps, renderedStep{Step: s, Code: code})
	}
	return v, nil
}
