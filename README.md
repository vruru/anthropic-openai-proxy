# Anthropic-to-OpenAI Translation Proxy (Go)

A Go rewrite of the Node.js proxy that translates between Anthropic Messages API and OpenAI Chat Completions format.

## Features

- **Bidirectional Translation**: Converts Anthropic requests to OpenAI format and translates responses back
- **Streaming Support**: Handles both streaming and non-streaming responses with dual-buffer strategy for tool arguments
- **Tool/Function Calling**: Full support for tool calls with JSON validation
- **Multi-modal Content**: Supports text and image content blocks
- **Zero External Dependencies**: Uses only Go standard library
- **CORS Enabled**: Allows all origins for easy integration

## Project Structure

```
anthropic-proxy-go/
├── go.mod           # Go module definition
├── main.go          # Entry point and server setup
├── config.go        # Configuration with environment variables
├── types.go         # All data structures (Anthropic & OpenAI)
├── translate.go     # Request/response translation logic
├── stream.go        # Streaming translation with dual-buffer strategy
├── handler.go       # HTTP handler for proxy endpoints
├── middleware.go    # CORS and logging middleware
└── Makefile         # Build and run commands
```

## Configuration

Environment variables:

- `PROXY_PORT` - Server port (default: 18900)
- `GATEWAY_URL` - OpenAI-compatible gateway URL (default: https://ai-gateway.happycapy.ai/api/v1/chat/completions)
- `GATEWAY_KEY` - Gateway API key (optional)

## Building

```bash
# Build binary
make build

# Run directly
make run

# Cross-compile for Linux amd64
make linux

# Clean build artifacts
make clean
```

## Usage

1. Start the proxy:
   ```bash
   export GATEWAY_KEY="your-api-key"
   ./anthropic-proxy
   ```

2. Send Anthropic-formatted requests to `POST /v1/messages`:
   ```bash
   curl -X POST http://localhost:18900/v1/messages \
     -H "Content-Type: application/json" \
     -d '{
       "model": "claude-3-5-sonnet-20241022",
       "max_tokens": 1024,
       "messages": [{"role": "user", "content": "Hello!"}]
     }'
   ```

## Translation Details

### Request Translation (Anthropic → OpenAI)

- **System messages**: Converted from string or array format to OpenAI system message
- **Messages**: Supports text, image, tool_use, and tool_result blocks
- **Tools**: Anthropic tool definitions mapped to OpenAI function format
- **Images**: Base64 data URLs preserved in multipart content

### Response Translation (OpenAI → Anthropic)

- **Content blocks**: Text and tool_use blocks constructed from OpenAI response
- **Finish reasons**: Mapped appropriately (tool_calls → tool_use, length → max_tokens)
- **Usage stats**: Token counts translated to Anthropic format

### Streaming Translation

- **Dual-buffer strategy**: Maintains both replace and append buffers for tool arguments
- **JSON validation**: Selects valid JSON from buffers at flush time
- **Text buffering**: Delays text output until [DONE] to handle tool-use responses correctly
- **SSE format**: Emits Anthropic-compatible Server-Sent Events

## Key Differences from Node.js Version

1. **Type Safety**: Strong typing with struct definitions instead of dynamic objects
2. **Error Handling**: Explicit error checking throughout
3. **Concurrency**: Go's native HTTP server with goroutines per request
4. **Standard Library**: No external dependencies (net/http, encoding/json)
5. **Middleware Pattern**: Composable middleware chain for request processing

## Timeouts

- Read timeout: 30 seconds
- Write timeout: 300 seconds (long for streaming)
- Idle timeout: 120 seconds

## API Compatibility

Fully compatible with Anthropic Messages API v1:
- POST /v1/messages
- POST /messages (also supported)

Supports all Anthropic features:
- System messages (string or array)
- Multi-turn conversations
- Tool/function calling
- Multi-modal content (text + images)
- Streaming responses
- Temperature, top_p, stop_sequences parameters
