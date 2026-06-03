import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("health route", () => {
  it("reports the frontend process as healthy", async () => {
    const response = GET();
    const body = await response.json();

    expect(response.status).toBe(200);
    expect(body).toEqual({ status: "healthy" });
  });
});
