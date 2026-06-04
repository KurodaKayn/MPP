import { collectDefaultMetrics, Gauge, Registry } from "prom-client";

export interface Metrics {
  registry: Registry;
}

export function createMetrics(): Metrics {
  const registry = new Registry();
  collectDefaultMetrics({
    prefix: "collab_",
    register: registry,
  });

  const serviceInfo = new Gauge({
    name: "collab_service_info",
    help: "Static collab-service runtime information.",
    labelNames: ["service"] as const,
    registers: [registry],
  });
  serviceInfo.set({ service: "collab-service" }, 1);

  return { registry };
}
