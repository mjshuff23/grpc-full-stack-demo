# AI Agent Arena

A full-stack gRPC learning demo where two local Ollama agents debate a prompt while the backend streams live turns and deterministic reliability scores to a TypeScript frontend.

The app is built to show the practical shape of modern protobuf RPC:

- Go backend with Connect-Go over `net/http`
- React + TypeScript frontend with generated protobuf client types
- Shared Protocol Buffers contract in `proto/arena/v1/arena.proto`
- Server streaming for live arena events
- Native gRPC proof path with `grpcurl`
- Local LLM calls through Ollama

## Prerequisites

Install Go, Buf, protobuf tooling, grpcurl, Node/npm, and Ollama. The recommended showcase models are:

```bash
ollama pull llama3.2:3b
ollama pull gemma3:4b
```

The backend falls back to these tiny smoke-test models if the larger models are not installed:

```bash
ollama pull qwen3:0.6b
ollama pull llama3.2:1b
```

If your shell does not already expose Go-installed tools, add this before running commands:

```bash
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
```

## Install

```bash
npm install --prefix frontend
cd backend && go mod download
```

## Generate Protobuf Code

```bash
make generate
```

This generates:

- Go protobuf and Connect handlers under `backend/gen`
- TypeScript protobuf client definitions under `frontend/src/gen`

## Run

Terminal 1:

```bash
ollama serve
```

If Ollama is already running as a service, this command may not be needed.

Terminal 2:

```bash
make backend
```

Terminal 3:

```bash
make frontend
```

Open the Vite URL, usually:

```text
http://localhost:5173
```

## Native gRPC Demo

With the backend running:

```bash
grpcurl -plaintext localhost:8080 list
grpcurl -plaintext localhost:8080 list arena.v1.ArenaService
grpcurl -plaintext localhost:8080 arena.v1.ArenaService/ListModels
```

You can also invoke the Connect endpoint as JSON:

```bash
curl \
  --header "Content-Type: application/json" \
  --data '{}' \
  http://localhost:8080/arena.v1.ArenaService/ListModels
```

## Test

```bash
make test
```

Backend tests cover scoring rules, Ollama HTTP parsing, model listing, and the `RunArena` server stream with a fake LLM. The frontend build typechecks generated protobuf client usage and creates a production bundle.

## Project Structure

```text
proto/       protobuf API contract and Buf config
backend/     Go Connect service, arena orchestration, Ollama adapter, scoring
frontend/    Vite React TypeScript app and generated protobuf client
```
