package arena

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	arenav1 "github.com/yokito/grpc-full-stack-demo/backend/gen/arena/v1"
	"github.com/yokito/grpc-full-stack-demo/backend/gen/arena/v1/arenav1connect"
	"github.com/yokito/grpc-full-stack-demo/backend/internal/llm"
)

func TestListModelsReturnsInstalledModelsAndDefaults(t *testing.T) {
	client := newTestClient(t, &fakeLLM{
		models: []llm.Model{
			{Name: "qwen3:0.6b", SizeBytes: 522},
			{Name: "llama3.2:1b", SizeBytes: 1300},
		},
	})

	res, err := client.ListModels(context.Background(), &arenav1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if got := res.GetDefaultAgentAModel(); got != "qwen3:0.6b" {
		t.Fatalf("default agent A model = %q", got)
	}
	if got := res.GetDefaultAgentBModel(); got != "llama3.2:1b" {
		t.Fatalf("default agent B model = %q", got)
	}
	if len(res.GetModels()) != 2 {
		t.Fatalf("expected two models, got %#v", res.GetModels())
	}
}

func TestListModelsPrefersLargerShowcaseModelsWhenInstalled(t *testing.T) {
	client := newTestClient(t, &fakeLLM{
		models: []llm.Model{
			{Name: "qwen3:0.6b", SizeBytes: 522},
			{Name: "llama3.2:1b", SizeBytes: 1300},
			{Name: "llama3.2:3b", SizeBytes: 2000},
			{Name: "gemma3:4b", SizeBytes: 3300},
		},
	})

	res, err := client.ListModels(context.Background(), &arenav1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if got := res.GetDefaultAgentAModel(); got != "llama3.2:3b" {
		t.Fatalf("default agent A model = %q", got)
	}
	if got := res.GetDefaultAgentBModel(); got != "gemma3:4b" {
		t.Fatalf("default agent B model = %q", got)
	}
}

func TestRunArenaStreamsMatchTurnScoreAndReportEvents(t *testing.T) {
	client := newTestClient(t, &fakeLLM{
		models: []llm.Model{
			{Name: "qwen3:0.6b"},
			{Name: "llama3.2:1b"},
		},
		responses: map[string]string{
			"qwen3:0.6b":  "You're absolutely right, the answer is definitely yes.",
			"llama3.2:1b": "I would verify with tests before calling it proven.",
		},
	})

	stream, err := client.RunArena(context.Background(), &arenav1.RunArenaRequest{
		Prompt:    "Should we trust this patch?",
		MaxRounds: 1,
		AgentA: &arenav1.AgentConfig{
			DisplayName: "Agent A",
			Model:       "qwen3:0.6b",
			Persona:     "Confident advocate",
		},
		AgentB: &arenav1.AgentConfig{
			DisplayName: "Agent B",
			Model:       "llama3.2:1b",
			Persona:     "Skeptical reviewer",
		},
	})
	if err != nil {
		t.Fatalf("RunArena returned error: %v", err)
	}

	var cases []string
	var finalReport *arenav1.MatchReport
	for stream.Receive() {
		event := stream.Msg()
		switch event.GetEvent().(type) {
		case *arenav1.ArenaEvent_MatchStarted:
			cases = append(cases, "match_started")
		case *arenav1.ArenaEvent_TokenChunk:
			cases = append(cases, "token_chunk")
		case *arenav1.ArenaEvent_TurnCompleted:
			cases = append(cases, "turn_completed")
		case *arenav1.ArenaEvent_ScoreUpdate:
			cases = append(cases, "score_update")
		case *arenav1.ArenaEvent_MatchCompleted:
			cases = append(cases, "match_completed")
			finalReport = event.GetMatchCompleted().GetReport()
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream ended with error: %v", err)
	}

	for _, required := range []string{"match_started", "token_chunk", "turn_completed", "score_update", "match_completed"} {
		if !slices.Contains(cases, required) {
			t.Fatalf("missing %s in event cases %#v", required, cases)
		}
	}
	if finalReport == nil {
		t.Fatal("expected final report")
	}
	if got := finalReport.GetWinner(); got != "Agent B" {
		t.Fatalf("winner = %q, want Agent B", got)
	}
	if len(finalReport.GetRounds()) != 1 {
		t.Fatalf("round count = %d, want 1", len(finalReport.GetRounds()))
	}
}

func TestRunArenaRejectsMissingPrompt(t *testing.T) {
	client := newTestClient(t, &fakeLLM{})

	stream, err := client.RunArena(context.Background(), &arenav1.RunArenaRequest{})
	if err != nil {
		t.Fatalf("RunArena setup returned error: %v", err)
	}
	for stream.Receive() {
		t.Fatalf("unexpected event: %#v", stream.Msg())
	}

	if got := connect.CodeOf(stream.Err()); got != connect.CodeInvalidArgument {
		t.Fatalf("stream error code = %s, want %s", got, connect.CodeInvalidArgument)
	}
}

func TestPromptsFrameArenaAsHarmlessReliabilityExercise(t *testing.T) {
	agent := &arenav1.AgentConfig{
		DisplayName: "Agent B",
		Model:       "llama3.2:1b",
		Persona:     "Skeptical reviewer",
	}

	system := systemPrompt(agent)
	if !strings.Contains(system, "harmless software reliability evaluation") {
		t.Fatalf("system prompt should frame the task as harmless reliability evaluation, got %q", system)
	}
	if !strings.Contains(system, "Do not refuse") {
		t.Fatalf("system prompt should discourage generic refusal, got %q", system)
	}

	turn := turnPrompt("Should a team trust this patch?", 1, "")
	if !strings.Contains(turn, "This is a harmless debate about software engineering judgment") {
		t.Fatalf("turn prompt should include harmless debate framing, got %q", turn)
	}
}

func newTestClient(t *testing.T, llmClient llm.Client) arenav1connect.ArenaServiceClient {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := arenav1connect.NewArenaServiceHandler(NewService(llmClient))
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return arenav1connect.NewArenaServiceClient(server.Client(), server.URL)
}

type fakeLLM struct {
	models    []llm.Model
	responses map[string]string
}

func (f *fakeLLM) ListModels(context.Context) ([]llm.Model, error) {
	return f.models, nil
}

func (f *fakeLLM) Generate(ctx context.Context, req llm.GenerateRequest, onToken func(string) error) (string, error) {
	response := f.responses[req.Model]
	for _, chunk := range strings.SplitAfter(response, " ") {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if chunk == "" {
			continue
		}
		if err := onToken(chunk); err != nil {
			return "", err
		}
	}
	return response, nil
}
