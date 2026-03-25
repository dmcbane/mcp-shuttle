BINARY = mcp-shuttle
TAGS = mcp_go_client_oauth

.PHONY: build test clean

build:
	go build -tags $(TAGS) -o $(BINARY) .

test:
	go test -tags $(TAGS) ./... -timeout 30s

clean:
	rm -f $(BINARY)
