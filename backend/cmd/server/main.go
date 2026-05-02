package main

import (
	"log"
	"net/http"
	"os"

	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	arenav1 "github.com/yokito/grpc-full-stack-demo/backend/gen/arena/v1"
	"github.com/yokito/grpc-full-stack-demo/backend/gen/arena/v1/arenav1connect"
	"github.com/yokito/grpc-full-stack-demo/backend/internal/arena"
	"github.com/yokito/grpc-full-stack-demo/backend/internal/llm"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	addr := env("ARENA_ADDR", "localhost:8080")
	ollamaBaseURL := env("OLLAMA_BASE_URL", "http://localhost:11434")

	arenaService := arena.NewService(llm.NewOllamaClient(ollamaBaseURL, http.DefaultClient))
	mux := http.NewServeMux()

	path, handler := arenav1connect.NewArenaServiceHandler(arenaService)
	mux.Handle(path, handler)

	checker := grpchealth.NewStaticChecker(arenav1connect.ArenaServiceName)
	mux.Handle(grpchealth.NewHandler(checker))

	reflector := grpcreflect.NewStaticReflector(
		arenav1connect.ArenaServiceName,
		grpchealth.HealthV1ServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	log.Printf("AI Agent Arena backend listening on http://%s", addr)
	log.Printf("Ollama base URL: %s", ollamaBaseURL)
	log.Printf("Registered protobuf file: %s", arenav1.File_arena_v1_arena_proto.Path())

	server := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(withCORS(mux), &http2.Server{}),
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Connect-Protocol-Version, Connect-Timeout-Ms, Content-Type, Grpc-Timeout, X-Grpc-Web, X-User-Agent")
		w.Header().Set("Access-Control-Expose-Headers", "Connect-Accept-Encoding, Connect-Content-Encoding, Grpc-Message, Grpc-Status, Grpc-Status-Details-Bin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
