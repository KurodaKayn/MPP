import { collectDefaultMetrics, Counter, Gauge, Histogram, Registry } from "prom-client";

export interface Metrics {
  activeConnections: Gauge<string>;
  activeDocuments: Gauge<string>;
  authDenials: Counter<string>;
  recordUpdateFlush(durationSeconds: number): void;
  registry: Registry;
  setActiveDocuments(count: number): void;
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

  const activeConnections = new Gauge({
    name: "collab_active_connections",
    help: "Active websocket connections handled by this collab-service instance.",
    registers: [registry],
  });

  const activeDocuments = new Gauge({
    name: "collab_active_documents",
    help: "Active collaborative documents loaded in this collab-service instance.",
    registers: [registry],
  });

  const authDenials = new Counter({
    name: "collab_auth_denials_total",
    help: "Total denied collab websocket authentication attempts.",
    labelNames: ["reason"] as const,
    registers: [registry],
  });

  const updateFlushLatency = new Histogram({
    name: "collab_update_flush_latency_seconds",
    help: "Latency for flushing pending Yjs updates to durable storage.",
    buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5],
    registers: [registry],
  });

  return {
    activeConnections,
    activeDocuments,
    authDenials,
    recordUpdateFlush(durationSeconds: number) {
      updateFlushLatency.observe(durationSeconds);
    },
    registry,
    setActiveDocuments(count: number) {
      activeDocuments.set(count);
    },
  };
}
