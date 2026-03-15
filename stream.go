package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StreamState maintains state during streaming translation
type StreamState struct {
	Started          bool
	BlockIndex       int
	ToolBlockStarted bool
	OutputTokens     int
	StopReason       string
	TextBuffer       string
	HasToolCalls     bool
	ToolArgsReplace  map[int]string
	ToolArgsAppend   map[int]string
	CurrentToolID    string
	CurrentToolIdx   int
}

func NewStreamState() *StreamState {
	return &StreamState{
		Started:         false,
		BlockIndex:      0,
		ToolBlockStarted: false,
		OutputTokens:    0,
		StopReason:      "end_turn",
		TextBuffer:      "",
		HasToolCalls:    false,
		ToolArgsReplace: make(map[int]string),
		ToolArgsAppend:  make(map[int]string),
	}
}

// SSE formats an event as Server-Sent Event
func SSE(eventType string, dataObj interface{}) string {
	data, _ := json.Marshal(dataObj)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}

// ResolveArgs chooses the best arguments from dual buffers via JSON validation
func ResolveArgs(replaceVal, appendVal string) string {
	if replaceVal == "" && appendVal == "" {
		return ""
	}

	// Try replace buffer first
	if replaceVal != "" {
		var test interface{}
		if err := json.Unmarshal([]byte(replaceVal), &test); err == nil {
			return replaceVal
		}
	}

	// Try append buffer
	if appendVal != "" {
		var test interface{}
		if err := json.Unmarshal([]byte(appendVal), &test); err == nil {
			return appendVal
		}
	}

	// Neither valid, return replace or append
	if replaceVal != "" {
		return replaceVal
	}
	return appendVal
}

// TranslateStreamChunk converts OpenAI SSE chunk to Anthropic SSE format
func TranslateStreamChunk(line, requestModel string, state *StreamState) string {
	if !strings.HasPrefix(line, "data: ") {
		return ""
	}

	data := strings.TrimSpace(line[6:])

	// Handle [DONE]
	if data == "[DONE]" {
		out := ""

		// Flush and close any open tool block
		if state.ToolBlockStarted {
			idx := state.CurrentToolIdx
			finalArgs := ResolveArgs(state.ToolArgsReplace[idx], state.ToolArgsAppend[idx])
			if len(finalArgs) > 0 {
				out += SSE("content_block_delta", ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: state.BlockIndex,
					Delta: DeltaBlock{
						Type:        "input_json_delta",
						PartialJSON: finalArgs,
					},
				})
			}
			out += SSE("content_block_stop", ContentBlockStopEvent{
				Type:  "content_block_stop",
				Index: state.BlockIndex,
			})
			state.BlockIndex++
			state.ToolBlockStarted = false
		}

		// Flush buffered text only if no tool calls were seen
		if strings.TrimSpace(state.TextBuffer) != "" && !state.HasToolCalls {
			out += SSE("content_block_start", ContentBlockStartEvent{
				Type:  "content_block_start",
				Index: state.BlockIndex,
				ContentBlock: ContentBlock{
					Type: "text",
					Text: "",
				},
			})
			out += SSE("content_block_delta", ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: state.BlockIndex,
				Delta: DeltaBlock{
					Type: "text_delta",
					Text: state.TextBuffer,
				},
			})
			out += SSE("content_block_stop", ContentBlockStopEvent{
				Type:  "content_block_stop",
				Index: state.BlockIndex,
			})
			state.BlockIndex++
		}

		// Send message_delta and message_stop
		out += SSE("message_delta", MessageDeltaEvent{
			Type: "message_delta",
			Delta: MessageDelta{
				StopReason:   state.StopReason,
				StopSequence: nil,
			},
			Usage: MessageDeltaUsage{
				OutputTokens: state.OutputTokens,
			},
		})
		out += SSE("message_stop", MessageStopEvent{
			Type: "message_stop",
		})

		return out
	}

	// Parse chunk
	var chunk OAIStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return ""
	}

	if len(chunk.Choices) == 0 {
		return ""
	}

	delta := chunk.Choices[0].Delta
	out := ""

	// Send message_start
	if !state.Started {
		state.Started = true
		state.BlockIndex = 0
		state.OutputTokens = 0
		state.TextBuffer = ""
		state.HasToolCalls = false
		state.ToolArgsReplace = make(map[int]string)
		state.ToolArgsAppend = make(map[int]string)

		msgID := chunk.ID
		if msgID == "" {
			msgID = fmt.Sprintf("msg_%d", time.Now().UnixNano()/1000000)
		}

		inputTokens := 0
		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
		}

		out += SSE("message_start", MessageStartEvent{
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
					InputTokens:  inputTokens,
					OutputTokens: 0,
				},
			},
		})
	}

	// Helper to flush tool block
	flushToolBlock := func() string {
		if !state.ToolBlockStarted {
			return ""
		}
		flush := ""
		idx := state.CurrentToolIdx
		finalArgs := ResolveArgs(state.ToolArgsReplace[idx], state.ToolArgsAppend[idx])
		if len(finalArgs) > 0 {
			flush += SSE("content_block_delta", ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: state.BlockIndex,
				Delta: DeltaBlock{
					Type:        "input_json_delta",
					PartialJSON: finalArgs,
				},
			})
		}
		flush += SSE("content_block_stop", ContentBlockStopEvent{
			Type:  "content_block_stop",
			Index: state.BlockIndex,
		})
		state.BlockIndex++
		state.ToolBlockStarted = false
		return flush
	}

	// Handle tool calls
	if len(delta.ToolCalls) > 0 {
		state.HasToolCalls = true
		state.TextBuffer = ""

		for _, tc := range delta.ToolCalls {
			tcIdx := tc.Index
			if tcIdx == 0 && tc.ID == "" && tc.Function.Name == "" {
				tcIdx = state.CurrentToolIdx
			}

			// New tool call
			if tc.Function.Name != "" {
				out += flushToolBlock()

				toolID := tc.ID
				if toolID == "" {
					toolID = fmt.Sprintf("toolu_%d", time.Now().UnixNano()/1000000)
				}

				state.CurrentToolID = toolID
				state.CurrentToolIdx = tcIdx
				state.StopReason = "tool_use"
				state.ToolBlockStarted = true
				state.ToolArgsReplace[tcIdx] = ""
				state.ToolArgsAppend[tcIdx] = ""

				out += SSE("content_block_start", ContentBlockStartEvent{
					Type:  "content_block_start",
					Index: state.BlockIndex,
					ContentBlock: ContentBlock{
						Type:  "tool_use",
						ID:    state.CurrentToolID,
						Name:  tc.Function.Name,
						Input: json.RawMessage("{}"),
					},
				})
			}

			// Accumulate arguments
			if tc.Function.Arguments != "" {
				state.ToolArgsReplace[tcIdx] = tc.Function.Arguments
				state.ToolArgsAppend[tcIdx] += tc.Function.Arguments
			}
		}
	}

	// Handle text content - buffer only
	if delta.Content != "" {
		state.TextBuffer += delta.Content
		state.OutputTokens++
	}

	// Handle finish reason
	if chunk.Choices[0].FinishReason != "" {
		fr := chunk.Choices[0].FinishReason
		switch fr {
		case "tool_calls", "function_call":
			state.StopReason = "tool_use"
		case "length":
			state.StopReason = "max_tokens"
		default:
			if state.HasToolCalls {
				state.StopReason = "tool_use"
			} else {
				state.StopReason = "end_turn"
			}
		}
	}

	if chunk.Usage != nil && chunk.Usage.CompletionTokens > 0 {
		state.OutputTokens = chunk.Usage.CompletionTokens
	}

	return out
}
