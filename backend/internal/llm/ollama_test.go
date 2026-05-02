package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOllamaClientListsModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"models":[{"name":"qwen3:0.6b","size":522000000},{"name":"llama3.2:1b","size":1300000000}]}`)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, server.Client())
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	want := []Model{
		{Name: "qwen3:0.6b", SizeBytes: 522000000},
		{Name: "llama3.2:1b", SizeBytes: 1300000000},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestOllamaClientStreamsGenerateResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"message":{"content":"first "},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"second"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"done":true}`)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, server.Client())
	var chunks []string
	response, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "qwen3:0.6b",
		System: "Be concise.",
		Prompt: "Debate this.",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if response != "first second" {
		t.Fatalf("response = %q, want %q", response, "first second")
	}
	if !reflect.DeepEqual(chunks, []string{"first ", "second"}) {
		t.Fatalf("chunks = %#v", chunks)
	}
}
