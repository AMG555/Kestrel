# SSE MCP Test Server

This is a test server for validating SSE-mode external MCP functionality.

## Usage

### 1. Start the test server

```bash
cd cmd/test-sse-mcp-server
go run main.go
```

The server starts at `http://127.0.0.1:8082` and exposes the following endpoints:
- `GET /sse` - SSE event stream endpoint
- `POST /message` - Message receiving endpoint

### 2. Add configuration in Kestrel

Add an external MCP configuration in the web UI using the following JSON:

```json
{
  "test-sse-mcp": {
    "transport": "sse",
    "url": "http://127.0.0.1:8082/sse",
    "description": "SSE MCP test server",
    "timeout": 30
  }
}
```

### 3. Test the tools

The test server provides two test tools:

1. **test_echo** - Echoes the input text back
   - Parameter: `text` (string) - the text to echo

2. **test_add** - Computes the sum of two numbers
   - Parameter: `a` (number) - first number
   - Parameter: `b` (number) - second number

## How it works

1. The client establishes an SSE connection via `GET /sse` to receive server-pushed events.
2. The client sends MCP protocol messages via `POST /message`.
3. The server processes each message and pushes the response back over the SSE connection.

## Logging

The server outputs the following log entries:
- SSE client connect / disconnect events
- Received requests (method name and ID)
- Tool call details
