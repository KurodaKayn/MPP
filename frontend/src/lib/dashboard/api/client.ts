import { formatBearerToken, getStoredAuthToken } from "../../auth/client";
import type { AITextStreamOptions } from "./types";

type ApiErrorResponse = {
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

async function getDashboardErrorMessage(response: Response) {
  const fallback = `请求失败 (${response.status})`;

  try {
    const body = (await response.json()) as ApiErrorResponse;
    return body.error?.message || body.error?.code || body.message || fallback;
  } catch {
    return fallback;
  }
}

export async function fetchDashboard<T>(
  path: string,
  init?: Omit<RequestInit, "headers" | "credentials">,
): Promise<T> {
  const headers = new Headers({
    Accept: "application/json",
  });
  const token = getStoredAuthToken();

  if (token) {
    headers.set("Authorization", formatBearerToken(token));
  }

  if (init?.body) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    throw new Error(await getDashboardErrorMessage(response));
  }

  return response.json() as Promise<T>;
}

export async function streamDashboardText(
  path: string,
  body: unknown,
  options: AITextStreamOptions = {},
) {
  const headers = new Headers({
    Accept: "text/markdown, text/plain, application/json",
    "Content-Type": "application/json",
  });
  const token = getStoredAuthToken();

  if (token) {
    headers.set("Authorization", formatBearerToken(token));
  }

  const response = await fetch(path, {
    body: JSON.stringify(body),
    credentials: "same-origin",
    headers,
    method: "POST",
    signal: options.signal,
  });

  if (!response.ok) {
    throw new Error(await getDashboardErrorMessage(response));
  }

  if (!response.body) {
    const text = await response.text();
    options.onChunk?.(text, text);
    return text;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let accumulated = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      const trailing = decoder.decode();
      if (trailing) {
        accumulated += trailing;
        options.onChunk?.(trailing, accumulated);
      }
      if (!accumulated.trim()) {
        throw new Error("AI 没有返回内容，请换个说法再试。");
      }
      return accumulated;
    }

    const chunk = decoder.decode(value, { stream: true });
    if (!chunk) {
      continue;
    }
    accumulated += chunk;
    options.onChunk?.(chunk, accumulated);
  }
}
