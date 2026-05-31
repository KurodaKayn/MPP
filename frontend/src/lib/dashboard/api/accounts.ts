import { fetchDashboard } from "./client";
import type {
  BrowserSession,
  CancelBrowserSessionResult,
  CompleteBrowserSessionResult,
  DouyinAccount,
  SaveWechatAccountInput,
  SaveXAccountInput,
  StartBrowserSessionResult,
  WechatAccount,
  WechatConnectionTestResult,
  XAccount,
  XConnectionTestResult,
  ZhihuAccount,
} from "./types";

export function getWechatAccount() {
  return fetchDashboard<WechatAccount>(
    "/api/user/dashboard/settings/wechat/account",
  );
}

export function saveWechatAccount(input: SaveWechatAccountInput) {
  return fetchDashboard<WechatAccount>(
    "/api/user/dashboard/settings/wechat/account",
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function testWechatConnection(input: SaveWechatAccountInput) {
  return fetchDashboard<WechatConnectionTestResult>(
    "/api/user/dashboard/settings/wechat/test",
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function getDouyinAccount() {
  return fetchDashboard<DouyinAccount>(
    "/api/user/dashboard/settings/douyin/account",
  );
}

export function getZhihuAccount() {
  return fetchDashboard<ZhihuAccount>(
    "/api/user/dashboard/settings/zhihu/account",
  );
}

export function startBrowserSession(platform: string) {
  return fetchDashboard<StartBrowserSessionResult>(
    `/api/user/dashboard/settings/platforms/${encodeURIComponent(platform)}/browser-session`,
    { method: "POST" },
  );
}

export function getBrowserSession(sessionId: string) {
  return fetchDashboard<BrowserSession>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}`,
  );
}

export function completeBrowserSession(sessionId: string) {
  return fetchDashboard<CompleteBrowserSessionResult>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}/complete`,
    { method: "POST" },
  );
}

export function cancelBrowserSession(sessionId: string) {
  return fetchDashboard<CancelBrowserSessionResult>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}`,
    { method: "DELETE" },
  );
}

export function getXAccount() {
  return fetchDashboard<XAccount>("/api/user/dashboard/settings/x/account");
}

export function saveXAccount(input: SaveXAccountInput) {
  return fetchDashboard<XAccount>("/api/user/dashboard/settings/x/account", {
    body: JSON.stringify(input),
    method: "PUT",
  });
}

export function testXConnection(input: SaveXAccountInput) {
  return fetchDashboard<XConnectionTestResult>(
    "/api/user/dashboard/settings/x/test",
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}
