export const authTokenNames = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
] as const;

export const primaryAuthTokenName = authTokenNames[0];

export function formatBearerToken(token: string) {
  return token.toLowerCase().startsWith("bearer ") ? token : `Bearer ${token}`;
}
