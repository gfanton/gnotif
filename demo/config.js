// Where the demo finds gnotif, the echo realm and its chain. These are the
// local setup of demo/README.md; a deployment sets gnotif.xyz and onyx:
// server "https://gnotif.xyz", chain { id: "onyx-1", name: "Gno.land onyx testnet",
// rpc: "https://rpc.onyx.testnets.gno.land:443" }, gnoweb "https://onyx.testnets.gno.land".

/** Base URL of the gnotif server. */
export const server = "http://localhost:8080";

/** Package path of the echo realm whose trigger the demo leads with. */
export const echo = "gno.land/r/dev/echo/v0";

/** The chain the realm runs on: its id, a name for the page and Adena, and its RPC. */
export const chain = { id: "dev", name: "gnodev local chain", rpc: "http://127.0.0.1:26657" };

/** Base URL of the gnoweb that renders the realm. */
export const gnoweb = "http://localhost:8888";
