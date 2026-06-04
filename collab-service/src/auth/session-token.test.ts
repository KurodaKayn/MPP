import { SignJWT } from "jose";
import { describe, expect, it } from "vitest";

import { JwtCollabAuthenticator } from "./session-token.js";

const secret = "collab-secret";
const documentId = "11111111-1111-4111-8111-111111111111";
const userId = "22222222-2222-4222-8222-222222222222";

async function signSessionToken(overrides: Record<string, unknown> = {}) {
  return new SignJWT({
    user_id: userId,
    document_id: documentId,
    role: "editor",
    purpose: "collab-session",
    ...overrides,
  })
    .setProtectedHeader({ alg: "HS256" })
    .setIssuer("mpp-backend")
    .setAudience("mpp-collab-service")
    .setIssuedAt()
    .setExpirationTime("5m")
    .sign(new TextEncoder().encode(secret));
}

describe("JwtCollabAuthenticator", () => {
  it("verifies a valid collab session token", async () => {
    const authenticator = new JwtCollabAuthenticator(secret);

    const session = await authenticator.verify(
      await signSessionToken(),
      documentId,
    );

    expect(session).toEqual({
      userId,
      documentId,
      role: "editor",
    });
  });

  it("rejects tokens for a different document", async () => {
    const authenticator = new JwtCollabAuthenticator(secret);

    await expect(
      authenticator.verify(
        await signSessionToken(),
        "33333333-3333-4333-8333-333333333333",
      ),
    ).rejects.toThrow("document mismatch");
  });

  it("requires the collab-session purpose", async () => {
    const authenticator = new JwtCollabAuthenticator(secret);

    await expect(
      authenticator.verify(
        await signSessionToken({ purpose: "web-session" }),
        documentId,
      ),
    ).rejects.toThrow();
  });
});
