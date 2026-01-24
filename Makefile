PROTO_DIR=proto
GO_OUT=gen/go
PY_OUT=gen/python

.PHONY: gen-proto
gen-proto:
	@mkdir -p $(GO_OUT) $(PY_OUT)
	@protoc -I $(PROTO_DIR) \
	  --go_out=$(GO_OUT) --go_opt=paths=source_relative \
	  --go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
	  $(PROTO_DIR)/hsar/v1/*.proto
	@python -m grpc_tools.protoc -I $(PROTO_DIR) \
	  --python_out=$(PY_OUT) \
	  --grpc_python_out=$(PY_OUT) \
	  $(PROTO_DIR)/hsar/v1/*.proto
	@echo "Proto generation complete."
