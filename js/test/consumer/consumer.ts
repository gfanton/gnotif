import { Gnotif, GnotifError, type GnotifNotification, type Trigger } from "gnotif";

const gnotif = new Gnotif({ network: "onyx" });

const triggers: Trigger[] = await gnotif.triggers("gno.land/r/dev/echo/v0");

const off = gnotif.on("notification", (n: GnotifNotification) => {
  console.log(n.link);
});
off();

try {
  await gnotif.setOptins(triggers.map((t) => ({ trigger: t.id, value: "" })));
} catch (e) {
  if (e instanceof GnotifError) {
    console.log(e.code);
  }
}
