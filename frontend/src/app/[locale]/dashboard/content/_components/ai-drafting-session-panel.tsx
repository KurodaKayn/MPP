"use client";

import {
  Archive,
  Bot,
  Check,
  Clock3,
  FileText,
  History,
  Loader2,
  MessageSquareText,
  PanelRightOpen,
  Play,
  Plus,
  Send,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import {
  archiveMockAIDraftingSession,
  createMockAIDraftingSession,
  listMockAIDraftingSessions,
  resumeMockAIDraftingSession,
  sendMockAIDraftingMessage,
  type AIDraftingArtifact,
  type AIDraftingSession,
  type AIDraftingSessionDetail,
  type ContinueAIDraftingSessionInput,
  type PublishPlatform,
  type StartAIDraftingSessionInput,
} from "@/lib/dashboard/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ContentValue } from "@/lib/content/types";

type AIDraftingSessionPanelProps = {
  canEdit?: boolean;
  content: ContentValue;
  projectId?: string;
  selectedPlatforms: PublishPlatform[];
  title: string;
};

type PanelState = "loading" | "ready" | "sending" | "archived";

export function AIDraftingSessionPanel({
  canEdit = true,
  content,
  projectId,
  selectedPlatforms,
  title,
}: AIDraftingSessionPanelProps) {
  const [state, setState] = useState<PanelState>("loading");
  const [sessions, setSessions] = useState<AIDraftingSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState("");
  const [detail, setDetail] = useState<AIDraftingSessionDetail | null>(null);
  const [message, setMessage] = useState("");
  const [selectedTab, setSelectedTab] = useState("messages");

  const canInteract = Boolean(canEdit && projectId);
  const activeSession = useMemo(
    () => sessions.find((session) => session.id === activeSessionId) ?? null,
    [activeSessionId, sessions],
  );
  const readablePlatforms =
    selectedPlatforms.length > 0 ? selectedPlatforms.join(", ") : "wechat";

  useEffect(() => {
    if (!projectId) {
      setState("ready");
      return;
    }

    let mounted = true;

    void (async () => {
      setState("loading");
      const response = await listMockAIDraftingSessions(projectId);
      if (!mounted) {
        return;
      }
      setSessions(response.items);
      setActiveSessionId(response.items[0]?.id || "");
      setDetail(
        response.items[0]
          ? buildDetailFromSession(
              response.items[0],
              content,
              title,
              readablePlatforms,
            )
          : null,
      );
      setState("ready");
    })();

    return () => {
      mounted = false;
    };
  }, [content, projectId, readablePlatforms, title]);

  useEffect(() => {
    if (!detail && activeSession) {
      setDetail(
        buildDetailFromSession(
          activeSession,
          content,
          title,
          readablePlatforms,
        ),
      );
    }
  }, [activeSession, content, detail, readablePlatforms, title]);

  const createSession = async () => {
    if (!projectId) {
      return;
    }
    const input: StartAIDraftingSessionInput = {
      message: "Start a drafting workspace for this project.",
      title,
    };
    const nextDetail = await createMockAIDraftingSession(projectId, input);
    setSessions((current) => [nextDetail.session, ...current]);
    setActiveSessionId(nextDetail.session.id);
    setDetail(nextDetail);
    setMessage("");
    setSelectedTab("messages");
    toast.success("Drafting session created");
  };

  const sendMessage = async () => {
    if (!projectId || !message.trim()) {
      return;
    }

    const input: ContinueAIDraftingSessionInput = {
      message: message.trim(),
    };
    setState("sending");
    let currentSession = activeSession;
    if (!currentSession) {
      const nextDetail = await createMockAIDraftingSession(projectId, {
        message: input.message,
        title,
      });
      currentSession = nextDetail.session;
      setSessions((current) => [nextDetail.session, ...current]);
      setActiveSessionId(nextDetail.session.id);
      setDetail(nextDetail);
      setMessage("");
      setSelectedTab("messages");
      setState("ready");
      toast.success("Drafting session created");
      return;
    }

    const nextDetail = await sendMockAIDraftingMessage(currentSession, input);
    setDetail(nextDetail);
    setSessions((current) =>
      current.map((session) =>
        session.id === currentSession.id
          ? {
              ...session,
              last_message_at: nextDetail.session.last_message_at,
              status: nextDetail.session.status,
              updated_at: nextDetail.session.updated_at,
            }
          : session,
      ),
    );
    setMessage("");
    setState(currentSession.status === "archived" ? "archived" : "ready");
    toast.success("Message added");
  };

  const archiveSession = async () => {
    if (!activeSession) {
      return;
    }

    const archived = await archiveMockAIDraftingSession(activeSession);
    setSessions((current) =>
      current.map((session) =>
        session.id === archived.id ? archived : session,
      ),
    );
    setActiveSessionId(archived.id);
    setDetail((current) =>
      current ? { ...current, session: archived } : current,
    );
    setState("archived");
    toast.success("Session archived");
  };

  const resumeSession = async () => {
    if (!activeSession) {
      return;
    }

    const resumed = await resumeMockAIDraftingSession(activeSession);
    setSessions((current) =>
      current.map((session) => (session.id === resumed.id ? resumed : session)),
    );
    setActiveSessionId(resumed.id);
    setDetail((current) =>
      current ? { ...current, session: resumed } : current,
    );
    setState("ready");
    toast.success("Session resumed");
  };

  const statusLabel =
    state === "loading"
      ? "Loading sessions"
      : state === "sending"
        ? "Thinking"
        : state === "archived"
          ? "Archived"
          : "Ready";

  return (
    <Card className="border-muted/60 shadow-sm">
      <CardHeader className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2 text-base">
              <PanelRightOpen className="size-4" />
              Drafting sessions
            </CardTitle>
            <CardDescription>
              Persistent drafting room for multi-turn context and reviewable
              artifacts.
            </CardDescription>
          </div>
          <Badge variant="secondary">{statusLabel}</Badge>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            className="gap-2"
            disabled={!canInteract || state === "loading"}
            onClick={() => void createSession()}
            size="sm"
          >
            <Plus className="size-4" />
            New Session
          </Button>
          <Button
            className="gap-2"
            disabled={!activeSession || activeSession.status === "archived"}
            onClick={() => void archiveSession()}
            size="sm"
            variant="outline"
          >
            <Archive className="size-4" />
            Archive
          </Button>
          <Button
            className="gap-2"
            disabled={!activeSession || activeSession.status === "active"}
            onClick={() => void resumeSession()}
            size="sm"
            variant="outline"
          >
            <Play className="size-4" />
            Resume
          </Button>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {!projectId ? (
          <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            Save the project before opening a session.
          </div>
        ) : null}

        <div className="grid gap-4 lg:grid-cols-[260px_minmax(0,1fr)]">
          <div className="space-y-3 rounded-lg border bg-muted/20 p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <History className="size-4" />
              Session list
            </div>
            <ScrollArea className="h-[260px] pr-2">
              <div className="space-y-2">
                {sessions.map((session) => (
                  <button
                    key={session.id}
                    className={cn(
                      "flex w-full flex-col gap-1 rounded-md border px-3 py-2 text-left text-sm transition-colors",
                      session.id === activeSessionId
                        ? "border-ring bg-background"
                        : "border-transparent bg-background/60 hover:border-border hover:bg-background",
                    )}
                    onClick={() => {
                      setActiveSessionId(session.id);
                      setDetail(
                        buildDetailFromSession(
                          session,
                          content,
                          title,
                          readablePlatforms,
                        ),
                      );
                    }}
                    type="button"
                  >
                    <span className="flex items-center justify-between gap-2">
                      <span className="font-medium">{session.title}</span>
                      <Badge variant="outline">{session.status}</Badge>
                    </span>
                    <span className="text-xs text-muted-foreground">
                      Updated {session.last_message_at}
                    </span>
                  </button>
                ))}
                {sessions.length === 0 ? (
                  <div className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
                    No session selected
                  </div>
                ) : null}
              </div>
            </ScrollArea>
          </div>

          <div className="space-y-4">
            <Tabs value={selectedTab} onValueChange={setSelectedTab}>
              <TabsList className="w-full justify-start">
                <TabsTrigger className="gap-2" value="messages">
                  <MessageSquareText className="size-4" />
                  Messages
                </TabsTrigger>
                <TabsTrigger className="gap-2" value="events">
                  <Clock3 className="size-4" />
                  Events
                </TabsTrigger>
                <TabsTrigger className="gap-2" value="artifacts">
                  <FileText className="size-4" />
                  Artifacts
                </TabsTrigger>
              </TabsList>

              <TabsContent className="mt-4" value="messages">
                <div className="space-y-3">
                  <div className="rounded-lg border bg-background">
                    <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                      <span className="flex items-center gap-2 font-medium">
                        <Bot className="size-4" />
                        Message history
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {readablePlatforms}
                      </span>
                    </div>
                    <ScrollArea className="h-[220px]">
                      <div className="space-y-3 p-3">
                        {(detail?.messages ?? []).map((item) => (
                          <div
                            key={item.id}
                            className={cn(
                              "max-w-[90%] rounded-lg border px-3 py-2 text-sm",
                              item.role === "user"
                                ? "ml-auto bg-muted/50"
                                : "bg-background",
                            )}
                          >
                            <div className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">
                              {item.role}
                            </div>
                            <div className="whitespace-pre-wrap">
                              {item.content}
                            </div>
                          </div>
                        ))}
                        {detail?.messages.length ? null : (
                          <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                            No messages yet. Send the first prompt to open a
                            drafting thread.
                          </div>
                        )}
                      </div>
                    </ScrollArea>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="drafting-message">Message</Label>
                    <Textarea
                      id="drafting-message"
                      disabled={!canInteract || state === "loading"}
                      onInput={(event) => setMessage(event.currentTarget.value)}
                      placeholder="Describe what the drafting room should do next"
                      value={message}
                    />
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-xs text-muted-foreground">
                        The shell keeps history locally until the backend
                        harness is ready.
                      </p>
                      <Button
                        className="gap-2"
                        disabled={
                          !canInteract || !message.trim() || state === "loading"
                        }
                        onClick={() => void sendMessage()}
                        size="sm"
                      >
                        {state === "sending" ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <Send className="size-4" />
                        )}
                        Send
                      </Button>
                    </div>
                  </div>
                </div>
              </TabsContent>

              <TabsContent className="mt-4" value="events">
                <div className="rounded-lg border bg-background">
                  <div className="border-b px-3 py-2 text-sm font-medium">
                    Timeline
                  </div>
                  <ScrollArea className="h-[290px]">
                    <div className="space-y-3 p-3">
                      {(detail?.events ?? []).map((event) => (
                        <div
                          key={event.id}
                          className="rounded-lg border bg-muted/20 p-3 text-sm"
                        >
                          <div className="flex items-center justify-between gap-2">
                            <div className="font-medium">{event.title}</div>
                            {event.status ? (
                              <Badge variant="outline">{event.status}</Badge>
                            ) : null}
                          </div>
                          {event.detail ? (
                            <p className="mt-2 text-sm text-muted-foreground">
                              {event.detail}
                            </p>
                          ) : null}
                          <div className="mt-2 text-xs text-muted-foreground">
                            {event.event_type} / {event.created_at}
                          </div>
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                </div>
              </TabsContent>

              <TabsContent className="mt-4" value="artifacts">
                <div className="rounded-lg border bg-background">
                  <div className="border-b px-3 py-2 text-sm font-medium">
                    Reviewable artifacts
                  </div>
                  <div className="space-y-3 p-3">
                    {(detail?.artifacts ?? []).map((artifact) => (
                      <ArtifactCard key={artifact.id} artifact={artifact} />
                    ))}
                    {detail?.artifacts.length ? null : (
                      <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                        No artifacts yet.
                      </div>
                    )}
                  </div>
                </div>
              </TabsContent>
            </Tabs>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function buildDetailFromSession(
  session: AIDraftingSession,
  content: ContentValue,
  title: string,
  platforms: string,
): AIDraftingSessionDetail {
  const createdAt = session.created_at;
  const prompt = content.text || content.html || "Original body";

  return {
    session,
    messages: [
      {
        content: `Drafting session opened for ${title}. Target platforms: ${platforms}.`,
        created_at: createdAt,
        id: `${session.id}-message-system`,
        role: "system",
        session_id: session.id,
      },
      {
        content: prompt,
        created_at: createdAt,
        id: `${session.id}-message-source`,
        role: "assistant",
        session_id: session.id,
      },
    ],
    events: [
      {
        created_at: createdAt,
        detail: "Current project content is ready for review.",
        event_type: "context",
        id: `${session.id}-event-context`,
        session_id: session.id,
        status: "completed",
        title: "Context loaded",
      },
      {
        created_at: createdAt,
        detail:
          "A future backend harness can stream tool calls and compact boundaries here.",
        event_type: "status",
        id: `${session.id}-event-stream`,
        session_id: session.id,
        status: "queued",
        title: "Stream endpoint",
      },
    ],
    artifacts: [
      {
        created_at: createdAt,
        id: `${session.id}-artifact-source`,
        kind: "source_patch",
        session_id: session.id,
        status: "proposed",
        summary: "Keep the argument but tighten the opening and transition.",
        target_platform: "wechat",
        title: "Source rewrite proposal",
      },
    ],
  };
}

function ArtifactCard({ artifact }: { artifact: AIDraftingArtifact }) {
  return (
    <div className="rounded-lg border bg-muted/20 p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="font-medium">{artifact.title}</div>
        <Badge variant="outline">{artifact.status}</Badge>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{artifact.summary}</p>
      <div className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
        <Check className="size-3.5" />
        <span>{artifact.kind}</span>
        {artifact.target_platform ? (
          <span>/ {artifact.target_platform}</span>
        ) : null}
      </div>
    </div>
  );
}
