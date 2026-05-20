PROTO_DIR=proto
GO_OUT=gen/go
PY_OUT=gen/python

export PATH := $(HOME)/go/bin:$(PATH)

.PHONY: proto gen-proto test lint up smoke

proto: gen-proto

gen-proto:
	@mkdir -p $(GO_OUT) $(PY_OUT)
	buf lint
	buf generate
	@echo "Proto generation complete."

test: proto
	go test -race ./...
	cd signal-engine && python3 -m pytest -q

lint: proto
	buf lint
	go vet ./...
	cd signal-engine && python3 -m ruff check .

up:
	docker compose up --build -d

smoke: up
	./scripts/smoke.sh