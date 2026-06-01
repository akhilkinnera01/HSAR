PROTO_DIR=proto
GO_OUT=gen/go
PY_OUT=gen/python

export PATH := $(HOME)/go/bin:$(PATH)

.PHONY: proto gen-proto test lint vuln up smoke smoke-ollama

proto: gen-proto

gen-proto:
	@mkdir -p $(GO_OUT) $(PY_OUT)
	buf lint
	buf generate
	python3 -m grpc_tools.protoc -I $(PROTO_DIR) \
	  --python_out=$(PY_OUT) \
	  --grpc_python_out=$(PY_OUT) \
	  $(PROTO_DIR)/hsar/v1/*.proto
	@echo "Proto generation complete."

PY_DEV=signal-engine/requirements-dev.txt

test: proto
	go test -race ./...
	pip install -q -r signal-engine/requirements.txt -r $(PY_DEV)
	cd signal-engine && PYTHONPATH=../gen/python python3 -m pytest -q

lint: proto
	buf lint
	go vet ./...
	pip install -q -r $(PY_DEV)
	cd signal-engine && python3 -m ruff check .

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

up:
	docker compose up --build -d

smoke: up
	./scripts/smoke.sh

smoke-ollama:
	./scripts/smoke-ollama.sh