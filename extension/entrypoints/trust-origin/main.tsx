import React from "react";
import { createRoot } from "react-dom/client";
import { ShieldCheck, ShieldX } from "lucide-react";
import "../../src/styles.css";
import { Button } from "../../src/components/ui/button";
import type { BackgroundMessage } from "../../src/types/messages";

function sendBackgroundMessage<T>(message: BackgroundMessage): Promise<T> {
  return browser.runtime.sendMessage(message);
}

function TrustOriginPage() {
  const origin = new URLSearchParams(location.search).get("origin") ?? "";
  const [status, setStatus] = React.useState<"idle" | "trusted" | "failed">(
    "idle",
  );
  const [error, setError] = React.useState("");

  const approve = async () => {
    try {
      await sendBackgroundMessage({ type: "origin.trust", origin });
      setStatus("trusted");
      setError("");
    } catch (nextError) {
      setStatus("failed");
      setError(
        nextError instanceof Error ? nextError.message : String(nextError),
      );
    }
  };

  const openMonitor = async () => {
    await browser.tabs.create({
      active: true,
      url: browser.runtime.getURL("/publish.html"),
    });
  };

  return (
    <main className="flex min-h-screen items-center justify-center px-5 py-8">
      <section className="w-full max-w-md rounded-md border border-zinc-200 bg-white p-5">
        <div className="flex items-center gap-3">
          {status === "failed" ? (
            <ShieldX className="text-red-600" size={22} />
          ) : (
            <ShieldCheck className="text-emerald-600" size={22} />
          )}
          <div>
            <h1 className="text-lg font-semibold text-zinc-950">
              Trust MPP Origin
            </h1>
            <p className="mt-1 text-sm text-zinc-500">
              Approve this origin before it can send publishing handoffs.
            </p>
          </div>
        </div>

        <div className="mt-5 rounded-md bg-zinc-50 p-3 text-sm text-zinc-700">
          <span className="break-all">{origin || "No origin supplied"}</span>
        </div>

        {status === "trusted" ? (
          <p className="mt-4 rounded-md bg-emerald-50 p-3 text-sm text-emerald-800">
            Origin trusted. MPP can now hand off extension publishing drafts.
          </p>
        ) : null}

        {error ? (
          <p className="mt-4 rounded-md bg-red-50 p-3 text-sm text-red-800">
            {error}
          </p>
        ) : null}

        <div className="mt-5 flex flex-wrap gap-2">
          <Button onClick={approve} disabled={!origin || status === "trusted"}>
            <ShieldCheck size={16} />
            Approve
          </Button>
          <Button variant="outline" onClick={openMonitor}>
            Open Monitor
          </Button>
        </div>
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <TrustOriginPage />
  </React.StrictMode>,
);
