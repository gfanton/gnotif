import { Gnotif, GnotifError, type GnotifNotification, type Trigger } from "gnotif";

const gnotif = new Gnotif({ network: "onyx" });

// A dapp passes options straight from its environment, where each may be unset.
declare const fromEnv: string | undefined;
declare const pageOnly: boolean | undefined;
new Gnotif({ network: fromEnv, server: fromEnv, serviceWorker: fromEnv, icon: fromEnv });

const triggers: Trigger[] = await gnotif.triggers("gno.land/r/dev/echo/v0");

const off = gnotif.on("notification", (n: GnotifNotification) => {
  console.log(n.link);
});
off();

try {
  await gnotif.setOptins(triggers.map((t) => ({ trigger: t.id, value: "", push: pageOnly })));
} catch (e) {
  if (e instanceof GnotifError) {
    console.log(e.code);
  }
}
