import { timingSafeEqual } from "node:crypto";

export function isInternalTokenAuthorized(
  authorization: string | undefined,
  secret: string | undefined,
): boolean {
  if (!authorization || !secret) {
    return false;
  }

  const prefix = "Bearer ";
  if (!authorization.startsWith(prefix)) {
    return false;
  }

  const token = Buffer.from(authorization.slice(prefix.length));
  const expected = Buffer.from(secret);
  return token.length === expected.length && timingSafeEqual(token, expected);
}
