// Where the demo finds gnotif, the echo realm and its chain, one entry per
// deployment: local is the setup of demo/README.md, onyx is demo.gnotif.xyz.

export type DemoConfig = {
  /** options for new Gnotif */
  gnotif: { server: string } | { network: string };
  /** package path of the echo realm whose trigger the demo leads with */
  echo: string;
  /** the chain the realm runs on: its id, a name for the page and Adena, and its RPC */
  chain: { id: string; name: string; rpc: string };
  /** base URL of the gnoweb that renders the realm */
  gnoweb: string;
};

const configs = {
  local: {
    gnotif: { server: "http://localhost:8080" },
    echo: "gno.land/r/dev/echo/v1",
    chain: { id: "dev", name: "gnodev local chain", rpc: "http://127.0.0.1:26657" },
    gnoweb: "http://localhost:8888",
  },
  onyx: {
    gnotif: { network: "onyx" },
    echo: "gno.land/r/nym-gfanton001/echo/v0",
    chain: { id: "onyx-1", name: "Gno.land onyx testnet", rpc: "https://rpc.onyx.testnets.gno.land:443" },
    gnoweb: "https://onyx.testnets.gno.land",
  },
} as const satisfies Record<string, DemoConfig>;

export function configFor(name: string): DemoConfig {
  switch (name) {
    case "local":
    case "onyx":
      return configs[name];
    default:
      throw new Error(`unknown network "${name}": use "local" or "onyx"`);
  }
}
