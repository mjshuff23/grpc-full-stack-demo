SHELL := /bin/bash
PATH := $(PATH):/usr/local/go/bin:$(HOME)/go/bin:$(CURDIR)/frontend/node_modules/.bin

.PHONY: generate test backend frontend

generate:
	buf lint
	buf generate

test:
	cd backend && go test ./...
	npm run build --prefix frontend

backend:
	cd backend && go run ./cmd/server

frontend:
	npm run dev --prefix frontend

