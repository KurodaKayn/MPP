import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

const config = loadConfig();
const app = await buildApp(config);

const shutdown = async (signal: NodeJS.Signals) => {
  app.log.info({ signal }, "stopping collab-service");
  await app.close();
};

process.once("SIGINT", shutdown);
process.once("SIGTERM", shutdown);

await app.listen({
  host: config.COLLAB_HOST,
  port: config.COLLAB_PORT,
});
