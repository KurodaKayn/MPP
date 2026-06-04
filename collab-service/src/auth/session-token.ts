import { jwtVerify } from "jose";
import { z } from "zod";

import type { CollabConfig } from "../config.js";

const CollabSessionPayloadSchema = z.object({
  user_id: z.string().uuid(),
  document_id: z.string().uuid(),
  role: z.enum(["editor", "viewer"]),
  purpose: z.literal("collab-session"),
});

export interface CollabSession {
  userId: string;
  documentId: string;
  role: "editor" | "viewer";
}

export interface CollabAuthenticator {
  verify(token: string, documentId?: string): Promise<CollabSession>;
}

export function createCollabAuthenticator(
  config: CollabConfig,
): CollabAuthenticator {
  return new JwtCollabAuthenticator(config.COLLAB_TOKEN_SECRET);
}

export class JwtCollabAuthenticator implements CollabAuthenticator {
  private readonly secret: Uint8Array;

  constructor(secret?: string) {
    if (!secret) {
      throw new Error("COLLAB_TOKEN_SECRET must be set");
    }
    this.secret = new TextEncoder().encode(secret);
  }

  async verify(token: string, documentId?: string): Promise<CollabSession> {
    const { payload } = await jwtVerify(token, this.secret, {
      issuer: "mpp-backend",
      audience: "mpp-collab-service",
    });
    const parsed = CollabSessionPayloadSchema.parse(payload);

    if (documentId && parsed.document_id !== documentId) {
      throw new Error("collab session token document mismatch");
    }

    return {
      userId: parsed.user_id,
      documentId: parsed.document_id,
      role: parsed.role,
    };
  }
}
