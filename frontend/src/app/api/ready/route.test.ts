import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("ready route", () => {
  it("reports the frontend process as ready", async () => {
    const response = GET();
    const body = await response.json();

    expect(response.status).toBe(200);
    expect(body).toEqual({ status: "ready" });
  });
});
