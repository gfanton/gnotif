package main

import (
	"html/template"
	"strings"
)

// step is one of the three integration steps shown on the landing page, a
// station on the path a notification takes. Text is the site's own copy and
// may carry inline code.
type step struct {
	Title    string
	Text     template.HTML
	Code     []snippet
	DocsHref string
}

// snippet is one code block of a step; Lang names its chroma lexer and is
// the label above it.
type snippet struct {
	Lang string
	Code string
}

var steps = [3]step{
	{
		Title: "Emit an event from your realm",
		Text:  "A realm with one function, <code>Notify</code>, emits a message and the address it is for.",
		Code: []snippet{{Lang: "gno", Code: `func Notify(cur realm, to address, msg string) {
	chain.Emit("Message", "to", to.String(), "msg", msg)
}`}},
		DocsHref: "/docs/getting-started/#1-emit-an-event",
	},
	{
		Title: "Set a trigger in the registry",
		Text:  "The realm opens its handle once, in <code>init</code>, and sets its trigger: which event to watch, who hears it, and what the notification says.",
		Code: []snippet{{Lang: "gno", Code: `import "` + registryPath + `"

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "message", gnotif.Trigger{
		Event: "Message", Param: "to", Title: "New message", Body: "{msg}", Link: "/",
	})
}`}},
		DocsHref: "/docs/getting-started/#2-set-the-trigger",
	},
	{
		Title: "Subscribe the browser from your page",
		Text:  "Install the client and serve its <code>sw.js</code> at your site's root. From a click, call <code>enable()</code>, then opt the browser in with the user's address. Call <code>connect()</code> too, and an open page gets each one live.",
		Code: []snippet{
			{Lang: "sh", Code: "npm install gnotif\ncp node_modules/gnotif/dist/sw.js public/sw.js"},
			{Lang: "js", Code: `import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ network: "onyx" });
// <you>: the address you deployed the realm from
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
if (!message) throw new Error("gnotif.xyz has not read this realm's trigger yet");

// button: a button on your page; address: the user's gno.land address
button.addEventListener("click", async () => {
  await gnotif.enable();
  await gnotif.setOptins([{ trigger: message.id, value: address }]);
});`},
		},
		DocsHref: "/docs/getting-started/#3-add-the-client-to-your-page",
	},
}

type renderedStep struct {
	Step   step
	Number int
	Code   []template.HTML
	// Last marks the end of the path, the browser: its station's dot is lit.
	Last bool
}

// scene is an event a realm emits and the notification it becomes. Label
// and App only show on the hero's signal line.
type scene struct {
	Label string
	Event string
	Attrs string
	Title string
	Body  string
	App   string
	Who   string
}

// unbroken is s with every short address, as in g1k7…x2q, held whole:
// browsers may break a line right after an ellipsis.
func (s scene) unbroken() scene {
	whole := strings.NewReplacer("…", "\u2060…\u2060")
	s.Attrs = whole.Replace(s.Attrs)
	s.Title = whole.Replace(s.Title)
	s.Who = whole.Replace(s.Who)
	return s
}

// scenes cycle on the hero's signal line. Amounts group their digits with a
// narrow no-break space and keep their unit with a no-break space.
var scenes = []scene{
	{Label: "pingpong · TurnPlayed", Event: "TurnPlayed", Attrs: "game=0000042 next=g1k7…x2q turn=7",
		Title: "Your turn", Body: "Game 0000042, turn 7", App: "pingpong", Who: "the player's browser"},
	{Label: "gnochat · Mentioned", Event: "Mentioned", Attrs: "channel=general by=g1zm…9ra who=g1k7…x2q",
		Title: "g1zm…9ra mentioned you", Body: "in #general", App: "gnochat", Who: "the member's browser"},
	{Label: "govdao · ProposalClosed", Event: "ProposalClosed", Attrs: "id=0000012 result=passed",
		Title: "Proposal 12 passed", Body: "Treasury budget, Q4", App: "govdao", Who: "every voter's browser"},
	{Label: "auctions · Outbid", Event: "Outbid", Attrs: "lot=0000309 by=g1p0…7cd was=g1k7…x2q",
		Title: "You were outbid", Body: "Lot 309, now 1\u202f250\u00a0GNOT", App: "auctions", Who: "the bidder's browser"},
	{Label: "vesting · Granted", Event: "Granted", Attrs: "account=g1k7…x2q unlock=1300000",
		Title: "New vesting grant", Body: "10\u202f000\u00a0GNOT, unlocking at block 1\u202f300\u202f000", App: "vesting", Who: "the grantee's browser"},
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
	v := landingView{Site: site, Flow: flow}
	for _, s := range scenes {
		v.Scenes = append(v.Scenes, s.unbroken())
	}
	for i, s := range steps {
		r := renderedStep{Step: s, Number: i + 1, Last: i == len(steps)-1}
		for _, c := range s.Code {
			code, err := highlight(c.Lang, c.Code)
			if err != nil {
				return landingView{}, err
			}
			r.Code = append(r.Code, code)
		}
		v.Steps = append(v.Steps, r)
	}
	return v, nil
}
