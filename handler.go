package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type ProxyHandler struct {
	config *Config
	client *http.Client
}

func NewProxyHandler(config *Config) *ProxyHandler {
	return &ProxyHandler{
		config: config,
		client: &http.Client{
			Timeout: 0, // No timeout for streaming
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Only handle /messages or /v1/messages endpoints
	if !strings.Contains(r.URL.Path, "/messages") {
		h.sendError(w, http.StatusNotFound, "Not found. Use POST /v1/messages")
		return
	}

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse Anthropic request
	var anthropicReq AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Translate to OpenAI format
	oaiReq, err := TranslateRequest(&anthropicReq)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("Translation error: %v", err))
		return
	}

	// Marshal OpenAI request
	oaiReqBytes, err := json.Marshal(oaiReq)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "Failed to marshal request")
		return
	}

	// Create request to gateway
	gatewayReq, err := http.NewRequest("POST", h.config.GatewayURL, bytes.NewReader(oaiReqBytes))
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "Failed to create gateway request")
		return
	}

	gatewayReq.Header.Set("Content-Type", "application/json")
	if h.config.GatewayKey != "" {
		gatewayReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.config.GatewayKey))
	}

	// Send request to gateway
	resp, err := h.client.Do(gatewayReq)
	if err != nil {
		log.Printf("[proxy] gateway error: %v", err)
		h.sendError(w, http.StatusBadGateway, fmt.Sprintf("Gateway error: %v", err))
		return
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		log.Printf("[proxy] gateway %d: %s", resp.StatusCode, string(errBody))
		h.sendError(w, resp.StatusCode, string(errBody))
		return
	}

	// Handle streaming vs non-streaming
	if anthropicReq.Stream {
		h.handleStreaming(w, resp, anthropicReq.Model)
	} else {
		h.handleNonStreaming(w, resp, anthropicReq.Model)
	}
}

func (h *ProxyHandler) handleNonStreaming(w http.ResponseWriter, resp *http.Response, requestModel string) {
	// Read entire response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(w, http.StatusBadGateway, "Failed to read gateway response")
		return
	}

	// Parse OpenAI response
	var oaiResp OAIResponse
	if err := json.Unmarshal(bodyBytes, &oaiResp); err != nil {
		h.sendError(w, http.StatusBadGateway, "Failed to parse gateway response")
		return
	}

	// Translate to Anthropic format
	anthropicResp := TranslateResponse(&oaiResp, requestModel)

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *ProxyHandler) handleStreaming(w http.ResponseWriter, resp *http.Response, requestModel string) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Flush headers
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	state := NewStreamState()
	reader := bufio.NewReader(resp.Body)
	buffer := ""

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[proxy] stream read error: %v", err)
				h.handleStreamError(w, state, requestModel)
			}
			break
		}

		buffer += line

		// Process complete lines
		if strings.HasSuffix(line, "\n") {
			lines := strings.Split(strings.TrimSuffix(buffer, "\n"), "\n")
			buffer = ""

			for _, l := range lines {
				trimmed := strings.TrimSpace(l)
				if trimmed == "" {
					continue
				}

				translated := TranslateStreamChunk(trimmed, requestModel, state)
				if translated != "" {
					w.Write([]byte(translated))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
			}
		}
	}

	// Process any remaining buffer
	if strings.TrimSpace(buffer) != "" {
		translated := TranslateStreamChunk(strings.TrimSpace(buffer), requestModel, state)
		if translated != "" {
			w.Write([]byte(translated))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

func (h *ProxyHandler) handleStreamError(w http.ResponseWriter, state *StreamState, requestModel string) {
	// Send error recovery events if stream was started
	if !state.Started {
		msgID := fmt.Sprintf("msg_err_%d", time.Now().UnixNano()/1000000)
		event := SSE("message_start", MessageStartEvent{
			Type: "message_start",
			Message: MessageStartEventMessage{
				ID:           msgID,
				Type:         "message",
				Role:         "assistant",
				Content:      []ContentBlock{},
				Model:        requestModel,
				StopReason:   nil,
				StopSequence: nil,
				Usage: MessageStartUsage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
		})
		w.Write([]byte(event))
	}

	if state.ToolBlockStarted || state.TextBuffer != "" {
		event := SSE("content_block_stop", ContentBlockStopEvent{
			Type:  "content_block_stop",
			Index: state.BlockIndex,
		})
		w.Write([]byte(event))
	}

	event := SSE("message_delta", MessageDeltaEvent{
		Type: "message_delta",
		Delta: MessageDelta{
			StopReason:   "end_turn",
			StopSequence: nil,
		},
		Usage: MessageDeltaUsage{
			OutputTokens: state.OutputTokens,
		},
	})
	w.Write([]byte(event))

	event = SSE("message_stop", MessageStopEvent{
		Type: "message_stop",
	})
	w.Write([]byte(event))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *ProxyHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Type: "error",
		Error: ErrorDetails{
			Type:    "api_error",
			Message: message,
		},
	})
}
