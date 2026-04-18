PROTO_DIR=proto
GO_OUT=gen/go
PY_OUT=gen/python

.PHONY: gen-proto gen-proto-go gen-proto-python lint vet build verify clean

gen-proto: gen-proto-go gen-proto-python

gen-proto-go:
	@mkdir -p $(GO_OUT)
	@protoc -I $(PROTO_DIR) \
	  --go_out=$(GO_OUT) --go_opt=paths=source_relative \
	  --go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
	  $(PROTO_DIR)/hsar/v1/*.proto

gen-proto-python:
	@mkdir -p $(PY_OUT)
	@python -m grpc_tools.protoc -I $(PROTO_DIR) \
	  --python_out=$(PY_OUT) \
	  --grpc_python_out=$(PY_OUT) \
	  $(PROTO_DIR)/hsar/v1/*.proto

vet: gen-proto
	@go vet ./...

lint: gen-proto
	@golangci-lint run --timeout=5m ./...
	@ruff check signal-engine/

build: gen-proto
	@go build ./cmd/proxy/

verify: lint vet build
	@echo "All checks passed."

clean:
	@rm -rf $(GO_OUT) $(PY_OUT) bin/
	@echo "Cleaned generated files."
