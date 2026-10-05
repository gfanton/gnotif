package main

import "html/template"

// step is one of the three integration steps shown on the landing page.
type step struct {
	Label    string
	Title    string
	Text     string
	Lang     string
	Code     string
	DocsHref string
}

var steps = [3]step{
	{
		Label:    "Emit",
		Title:    "Emit an event from your realm",
		Text:     "pingpong, the demo game, names the next player on every turn.",
		Lang:     "go",
		Code:     `chain.Emit("TurnPlayed", "game", g.ID, "next", g.Next.String(), "turn", strconv.Itoa(g.Turn))`,
		DocsHref: "/docs/getting-started/#1-emit-an-event",
	},
	{
		Label: "Declare",
		Title: "Declare a trigger in the registry",
		Text:  "Declared by the realm itself, the trigger is verified. It notifies the browsers that opted in with the address in the event's next attribute.",
		Lang:  "go",
		Code: `import "` + registryPath + `"

var declared bool

func DeclareTriggers(cur realm) {
	if declared {
		panic("triggers already declared")
	}
	declared = true
	gnotif.Declare(cross(cur), cur.PkgPath(), "TurnPlayed", "", "next",
		"Your turn", "Game {game}, turn {turn}", "/?game={game}")
}`,
		DocsHref: "/docs/getting-started/#2-declare-a-trigger",
	},
	{
		Label: "Subscribe",
		Title: "Subscribe the browser from your page",
		Text:  "Install the client, serve its service worker from your origin, and call enable() from a click.",
		Lang:  "js",
		Code: `import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ server: "` + serverURL + `" });
const yourTurn = (await gnotif.triggers()).find(
  (t) =>
    t.target === "` + pingpongPath + `" &&
    t.event === "TurnPlayed" &&
    t.verified,
);

button.addEventListener("click", async () => {
  await gnotif.enable();
  await gnotif.setOptins([{ trigger: yourTurn.id, value: playerAddress }]);
});`,
		DocsHref: "/docs/getting-started/#3-add-the-client-to-the-page",
	},
}

type renderedStep struct {
	Step step
	Code template.HTML
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
	{Name: "gnotifd", Note: "matches events, one SQLite file"},
	{Name: "push service", Note: "Web Push, the browser vendor's"},
	{Name: "sw.js", Note: "shows the notification, on the dapp's origin", Dashed: true},
}

// status is what version 0 leaves out.
var status = []string{
	"abuse limits: caps per IP, a send budget per declarer, and expiring subscriptions that stop re-registering;",
	"monitoring and a health endpoint;",
	"configuration through environment variables, beyond the VAPID keys;",
	"mainnet.",
}

type landingView struct {
	Site     siteView
	Steps    []renderedStep
	Flow     []flowNode
	Status   []string
	Registry string
}

func newLandingView() (landingView, error) {
	v := landingView{Site: site, Flow: flow, Status: status, Registry: registryPath}
	for _, s := range steps {
		code, err := highlight(s.Lang, s.Code)
		if err != nil {
			return landingView{}, err
		}
		v.Steps = append(v.Steps, renderedStep{Step: s, Code: code})
	}
	return v, nil
}
