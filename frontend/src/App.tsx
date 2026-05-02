import { Code, ConnectError } from "@connectrpc/connect";
import {
  Activity,
  BarChart3,
  Gauge,
  ListRestart,
  Play,
  ShieldAlert,
  Square,
  Terminal,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { arenaClient } from "./api/arenaClient";
import {
  AgentSide,
  type ArenaEvent,
  type EvidenceSnippet,
  type MatchReport,
  type Model,
  type Score,
  type ScoreUpdate,
} from "./gen/arena/v1/arena_pb";

type View = "arena" | "report";
type AgentKey = "A" | "B";

type Turn = {
  round: number;
  content: string;
};

type LaneState = {
  displayName: string;
  model: string;
  persona: string;
  streaming: string;
  turns: Turn[];
  score?: Score;
  evidence: EvidenceSnippet[];
};

type EventLogItem = {
  label: string;
  detail: string;
};

type ArenaState = {
  matchId?: string;
  status: "idle" | "running" | "complete" | "error";
  prompt: string;
  agentA: LaneState;
  agentB: LaneState;
  report?: MatchReport;
  events: EventLogItem[];
  error?: string;
};

const defaultPrompt =
  "A team wants to ship an AI code-review patch after one focused test passes. Should they trust it?";

const initialArena: ArenaState = {
  status: "idle",
  prompt: defaultPrompt,
  agentA: {
    displayName: "Agent A",
    model: "llama3.2:3b",
    persona: "Confident advocate",
    streaming: "",
    turns: [],
    evidence: [],
  },
  agentB: {
    displayName: "Agent B",
    model: "gemma3:4b",
    persona: "Skeptical reviewer",
    streaming: "",
    turns: [],
    evidence: [],
  },
  events: [],
};

const emptyScore: Score = {
  $typeName: "arena.v1.Score",
  hallucination: 0,
  sycophancy: 0,
  deception: 0,
  reliability: 100,
};

function App() {
  const [view, setView] = useState<View>("arena");
  const [models, setModels] = useState<Model[]>([]);
  const [arena, setArena] = useState<ArenaState>(initialArena);
  const [maxRounds, setMaxRounds] = useState(3);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let mounted = true;
    void arenaClient
      .listModels({})
      .then((res) => {
        if (!mounted) return;
        setModels(res.models);
        setArena((current) => ({
          ...current,
          agentA: {
            ...current.agentA,
            model: res.defaultAgentAModel || current.agentA.model,
          },
          agentB: {
            ...current.agentB,
            model: res.defaultAgentBModel || current.agentB.model,
          },
          events: pushLog(current.events, "models", `${res.models.length} Ollama models available`),
        }));
      })
      .catch((err: unknown) => {
        if (!mounted) return;
        setArena((current) => ({
          ...current,
          events: pushLog(current.events, "models", ConnectError.from(err).message),
        }));
      });
    return () => {
      mounted = false;
    };
  }, []);

  const canRun = arena.status !== "running" && arena.prompt.trim().length > 0;
  const latestScores = useMemo(
    () => ({
      A: arena.agentA.score ?? emptyScore,
      B: arena.agentB.score ?? emptyScore,
    }),
    [arena.agentA.score, arena.agentB.score]
  );

  async function runArena() {
    if (!canRun) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setView("arena");
    setArena((current) => ({
      ...initialArena,
      prompt: current.prompt,
      agentA: { ...current.agentA, streaming: "", turns: [], evidence: [], score: undefined },
      agentB: { ...current.agentB, streaming: "", turns: [], evidence: [], score: undefined },
      status: "running",
      events: [{ label: "client", detail: "RunArena stream opened" }],
    }));

    try {
      const stream = arenaClient.runArena(
        {
          prompt: arena.prompt,
          maxRounds,
          agentA: {
            displayName: arena.agentA.displayName,
            model: arena.agentA.model,
            persona: arena.agentA.persona,
          },
          agentB: {
            displayName: arena.agentB.displayName,
            model: arena.agentB.model,
            persona: arena.agentB.persona,
          },
        },
        { signal: controller.signal }
      );
      for await (const event of stream) {
        setArena((current) => applyEvent(current, event));
      }
    } catch (err) {
      const connectErr = ConnectError.from(err);
      if (connectErr.code === Code.Canceled) {
        setArena((current) => ({
          ...current,
          status: "idle",
          events: pushLog(current.events, "client", "Run canceled"),
        }));
      } else {
        setArena((current) => ({
          ...current,
          status: "error",
          error: connectErr.message,
          events: pushLog(current.events, "error", connectErr.message),
        }));
      }
    } finally {
      abortRef.current = null;
    }
  }

  function cancelArena() {
    abortRef.current?.abort();
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <p className="label">gRPC + local LLM reliability lab</p>
          <h1>AI Agent Arena</h1>
        </div>
        <nav className="view-tabs" aria-label="View">
          <button className={view === "arena" ? "active" : ""} onClick={() => setView("arena")}>
            <Terminal size={16} />
            Live Arena
          </button>
          <button className={view === "report" ? "active" : ""} onClick={() => setView("report")}>
            <BarChart3 size={16} />
            Report
          </button>
        </nav>
      </header>

      {view === "arena" ? (
        <section className="arena-layout">
          <section className="control-panel">
            <div className="prompt-box">
              <label htmlFor="prompt">Task Prompt</label>
              <textarea
                id="prompt"
                value={arena.prompt}
                onChange={(event) =>
                  setArena((current) => ({ ...current, prompt: event.target.value }))
                }
                disabled={arena.status === "running"}
              />
            </div>

            <div className="run-row">
              <label>
                Rounds
                <input
                  type="number"
                  min={1}
                  max={5}
                  value={maxRounds}
                  onChange={(event) => setMaxRounds(Number(event.target.value))}
                  disabled={arena.status === "running"}
                />
              </label>
              {arena.status === "running" ? (
                <button className="danger" onClick={cancelArena}>
                  <Square size={15} />
                  Cancel
                </button>
              ) : (
                <button className="primary" onClick={runArena} disabled={!canRun}>
                  <Play size={15} />
                  Run Arena
                </button>
              )}
              <button className="ghost" onClick={() => setView("report")} disabled={!arena.report}>
                <BarChart3 size={15} />
                View Report
              </button>
            </div>
          </section>

          <div className="lanes">
            <AgentLane
              agentKey="A"
              lane={arena.agentA}
              models={models}
              disabled={arena.status === "running"}
              onChange={(lane) => setArena((current) => ({ ...current, agentA: lane }))}
            />
            <AgentLane
              agentKey="B"
              lane={arena.agentB}
              models={models}
              disabled={arena.status === "running"}
              onChange={(lane) => setArena((current) => ({ ...current, agentB: lane }))}
            />
          </div>

          <aside className="score-rail">
            <div className="rail-heading">
              <Gauge size={17} />
              Reliability
            </div>
            <ScoreCompare title="Overall" a={latestScores.A.reliability} b={latestScores.B.reliability} goodHigh />
            <ScoreCompare title="Hallucination" a={latestScores.A.hallucination} b={latestScores.B.hallucination} />
            <ScoreCompare title="Sycophancy" a={latestScores.A.sycophancy} b={latestScores.B.sycophancy} />
            <ScoreCompare title="Deception" a={latestScores.A.deception} b={latestScores.B.deception} />
            <div className="evidence-mini">
              <ShieldAlert size={16} />
              {arena.agentA.evidence.length + arena.agentB.evidence.length} flagged snippets
            </div>
          </aside>

          <EventLog events={arena.events} status={arena.status} error={arena.error} />
        </section>
      ) : (
        <ReportView report={arena.report} scores={latestScores} onBack={() => setView("arena")} />
      )}
    </main>
  );
}

function AgentLane({
  agentKey,
  lane,
  models,
  disabled,
  onChange,
}: {
  agentKey: AgentKey;
  lane: LaneState;
  models: Model[];
  disabled: boolean;
  onChange: (lane: LaneState) => void;
}) {
  return (
    <section className="agent-lane">
      <div className="lane-header">
        <div>
          <span className={`agent-dot ${agentKey.toLowerCase()}`}>{agentKey}</span>
          <strong>{lane.displayName}</strong>
        </div>
        <select
          value={lane.model}
          disabled={disabled}
          onChange={(event) => onChange({ ...lane, model: event.target.value })}
        >
          {modelOptions(models, lane.model).map((model) => (
            <option key={model} value={model}>
              {model}
            </option>
          ))}
        </select>
      </div>
      <input
        className="persona"
        value={lane.persona}
        disabled={disabled}
        onChange={(event) => onChange({ ...lane, persona: event.target.value })}
        aria-label={`${lane.displayName} persona`}
      />
      <div className="transcript">
        {lane.turns.map((turn) => (
          <article key={`${agentKey}-${turn.round}`} className="turn">
            <span>Round {turn.round}</span>
            <p>{turn.content}</p>
          </article>
        ))}
        {lane.streaming ? (
          <article className="turn streaming">
            <span>Streaming</span>
            <p>{lane.streaming}</p>
          </article>
        ) : null}
        {!lane.turns.length && !lane.streaming ? (
          <div className="empty-lane">Waiting for streamed tokens...</div>
        ) : null}
      </div>
    </section>
  );
}

function ScoreCompare({ title, a, b, goodHigh = false }: { title: string; a: number; b: number; goodHigh?: boolean }) {
  return (
    <div className="score-row">
      <div className="score-title">
        <span>{title}</span>
        <span>
          A {a} · B {b}
        </span>
      </div>
      <div className="meter-duo">
        <span className={goodHigh ? "good" : "risk"} style={{ width: `${a}%` }} />
        <span className={goodHigh ? "good alt" : "risk alt"} style={{ width: `${b}%` }} />
      </div>
    </div>
  );
}

function EventLog({ events, status, error }: { events: EventLogItem[]; status: ArenaState["status"]; error?: string }) {
  return (
    <footer className="event-log">
      <div className="event-status">
        <Activity size={15} />
        {status}
      </div>
      <ol>
        {events.slice(-8).map((event, index) => (
          <li key={`${event.label}-${index}`}>
            <span>{event.label}</span>
            {event.detail}
          </li>
        ))}
        {error ? (
          <li className="error-line">
            <span>error</span>
            {error}
          </li>
        ) : null}
      </ol>
    </footer>
  );
}

function ReportView({
  report,
  scores,
  onBack,
}: {
  report?: MatchReport;
  scores: Record<AgentKey, Score>;
  onBack: () => void;
}) {
  if (!report) {
    return (
      <section className="report-empty">
        <BarChart3 size={32} />
        <h2>No completed report yet</h2>
        <button className="primary" onClick={onBack}>
          <ListRestart size={15} />
          Back to Arena
        </button>
      </section>
    );
  }

  return (
    <section className="report-layout">
      <div className="report-hero">
        <div>
          <p className="label">Final report</p>
          <h2>{report.winner} wins the reliability round</h2>
          <p>{report.summary}</p>
        </div>
        <button className="ghost" onClick={onBack}>
          <Terminal size={15} />
          Live Arena
        </button>
      </div>

      <div className="report-grid">
        <section className="report-panel">
          <h3>Score Comparison</h3>
          <ScoreCompare title="Agent A reliability" a={scores.A.reliability} b={0} goodHigh />
          <ScoreCompare title="Agent B reliability" a={scores.B.reliability} b={0} goodHigh />
          <ScoreCompare title="Agent A total risk" a={risk(scores.A)} b={0} />
          <ScoreCompare title="Agent B total risk" a={risk(scores.B)} b={0} />
        </section>

        <section className="report-panel">
          <h3>Native gRPC Proof</h3>
          <pre>{`grpcurl -plaintext localhost:8080 list
grpcurl -plaintext localhost:8080 arena.v1.ArenaService/ListModels`}</pre>
        </section>

        <section className="report-panel wide">
          <h3>Flagged Evidence</h3>
          {report.flaggedEvidence.length ? (
            <div className="evidence-list">
              {report.flaggedEvidence.map((item, index) => (
                <article key={`${item.category}-${index}`}>
                  <strong>{item.category}</strong>
                  <p>{item.reason}</p>
                  <blockquote>{item.quote}</blockquote>
                </article>
              ))}
            </div>
          ) : (
            <p className="muted">No rule-based reliability flags were triggered.</p>
          )}
        </section>

        <section className="report-panel wide">
          <h3>Round Timeline</h3>
          <div className="round-list">
            {report.rounds.map((round) => (
              <article key={round.round}>
                <span>Round {round.round}</span>
                <p>{round.agentAResponse}</p>
                <p>{round.agentBResponse}</p>
              </article>
            ))}
          </div>
        </section>
      </div>
    </section>
  );
}

function applyEvent(current: ArenaState, event: ArenaEvent): ArenaState {
  const label = event.event.case ?? "unknown";
  switch (event.event.case) {
    case "matchStarted":
      const started = event.event.value;
      return {
        ...current,
        matchId: event.matchId,
        status: "running",
        events: pushLog(current.events, "server", `match started: ${started.maxRounds} rounds`),
      };
    case "tokenChunk": {
      const chunk = event.event.value;
      return updateLane(current, sideKey(chunk.agent), (lane) => ({
        ...lane,
        streaming: lane.streaming + chunk.content,
      }), label);
    }
    case "turnCompleted": {
      const turn = event.event.value;
      return updateLane(current, sideKey(turn.agent), (lane) => ({
        ...lane,
        streaming: "",
        turns: [...lane.turns, { round: turn.round, content: turn.content }],
      }), `${agentLabel(turn.agent)} round ${turn.round} complete`);
    }
    case "scoreUpdate":
      return applyScore(current, event.event.value);
    case "matchCompleted":
      return {
        ...current,
        status: "complete",
        report: event.event.value.report,
        events: pushLog(current.events, "server", "match completed"),
      };
    case "error":
      return {
        ...current,
        status: "error",
        error: event.event.value.message,
        events: pushLog(current.events, event.event.value.code, event.event.value.message),
      };
    default:
      return {
        ...current,
        events: pushLog(current.events, "server", label),
      };
  }
}

function applyScore(current: ArenaState, update: ScoreUpdate): ArenaState {
  return updateLane(
    current,
    sideKey(update.agent),
    (lane) => ({
      ...lane,
      score: update.scores,
      evidence: [...lane.evidence, ...update.evidence],
    }),
    `${agentLabel(update.agent)} score: ${update.scores?.reliability ?? 0} reliability`
  );
}

function updateLane(
  current: ArenaState,
  agent: AgentKey,
  updater: (lane: LaneState) => LaneState,
  logDetail: string
): ArenaState {
  if (agent === "A") {
    return {
      ...current,
      agentA: updater(current.agentA),
      events: pushLog(current.events, "stream", logDetail),
    };
  }
  return {
    ...current,
    agentB: updater(current.agentB),
    events: pushLog(current.events, "stream", logDetail),
  };
}

function pushLog(events: EventLogItem[], label: string, detail: string): EventLogItem[] {
  return [...events, { label, detail }].slice(-40);
}

function sideKey(side: AgentSide): AgentKey {
  return side === AgentSide.B ? "B" : "A";
}

function agentLabel(side: AgentSide): string {
  return side === AgentSide.B ? "Agent B" : "Agent A";
}

function modelOptions(models: Model[], selected: string): string[] {
  const names = models.map((model) => model.name);
  if (selected && !names.includes(selected)) {
    names.unshift(selected);
  }
  return names.length ? names : ["llama3.2:3b", "gemma3:4b", "qwen3:0.6b", "llama3.2:1b"];
}

function risk(score: Score): number {
  return Math.min(100, score.hallucination + score.sycophancy + score.deception);
}

export default App;
