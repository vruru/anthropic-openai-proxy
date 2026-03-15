package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// TranslateRequest converts Anthropic request to OpenAI format
func TranslateRequest(body *AnthropicRequest) (*OAIRequest, error) {
	messages := []OAIMessage{}

	// Handle system message
	if len(body.System) > 0 {
		systemText := ""

		// Try parsing as string first
		var systemStr string
		if err := json.Unmarshal(body.System, &systemStr); err == nil {
			systemText = systemStr
		} else {
			// Try parsing as array of blocks
			var systemBlocks []ContentBlock
			if err := json.Unmarshal(body.System, &systemBlocks); err == nil {
				for _, block := range systemBlocks {
					if block.Text != "" {
						if systemText != "" {
							systemText += "\n"
						}
						systemText += block.Text
					}
				}
			}
		}

		if systemText != "" {
			messages = append(messages, OAIMessage{
				Role:    "system",
				Content: json.RawMessage(fmt.Sprintf(`"%s"`, escapeJSON(systemText))),
			})
		}
	}

	// Handle messages
	for _, msg := range body.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}

		// Try parsing as string
		var contentStr string
		if err := json.Unmarshal(msg.Content, &contentStr); err == nil {
			messages = append(messages, OAIMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
			continue
		}

		// Parse as array of content blocks
		var blocks []ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}

		var textParts []string
		var imageParts []interface{}
		var toolCalls []OAIToolCall
		var toolResults []OAIMessage

		for _, block := range blocks {
			switch block.Type {
			case "text":
				textParts = append(textParts, block.Text)
			case "image":
				if block.Source != nil {
					imageURL := fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data)
					imageParts = append(imageParts, map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageURL,
						},
					})
				}
			case "tool_use":
				toolCalls = append(toolCalls, OAIToolCall{
					ID:   block.ID,
					Type: "function",
					Function: OAIFunction{
						Name:      block.Name,
						Arguments: string(block.Input),
					},
				})
			case "tool_result":
				resultContent := ""
				// Try parsing as string
				var contentStr string
				if err := json.Unmarshal(block.Content, &contentStr); err == nil {
					resultContent = contentStr
				} else {
					// Try parsing as array
					var contentBlocks []ContentBlock
					if err := json.Unmarshal(block.Content, &contentBlocks); err == nil {
						for i, cb := range contentBlocks {
							if i > 0 {
								resultContent += "\n"
							}
							resultContent += cb.Text
						}
					}
				}

				contentJSON, _ := json.Marshal(resultContent)
				toolResults = append(toolResults, OAIMessage{
					Role:       "tool",
					ToolCallID: block.ToolUseID,
					Content:    contentJSON,
				})
			}
		}

		// Build message based on what we found
		if len(toolCalls) > 0 {
			content := json.RawMessage("null")
			if len(textParts) > 0 {
				combined := ""
				for _, p := range textParts {
					combined += p
				}
				contentJSON, _ := json.Marshal(combined)
				content = contentJSON
			}
			messages = append(messages, OAIMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			})
		} else if len(toolResults) > 0 {
			messages = append(messages, toolResults...)
		} else if len(imageParts) > 0 || len(textParts) > 0 {
			// Build multipart content
			parts := []interface{}{}
			for _, text := range textParts {
				parts = append(parts, map[string]interface{}{
					"type": "text",
					"text": text,
				})
			}
			parts = append(parts, imageParts...)

			if len(imageParts) > 0 {
				// Multipart format
				contentJSON, _ := json.Marshal(parts)
				messages = append(messages, OAIMessage{
					Role:    msg.Role,
					Content: contentJSON,
				})
			} else {
				// Simple text format
				combined := ""
				for _, p := range textParts {
					combined += p
				}
				contentJSON, _ := json.Marshal(combined)
				messages = append(messages, OAIMessage{
					Role:    msg.Role,
					Content: contentJSON,
				})
			}
		}
	}

	// Build OpenAI request
	maxTokens := body.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	req := &OAIRequest{
		Model:     body.Model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	// Handle tools
	if len(body.Tools) > 0 {
		oaiTools := make([]OAITool, len(body.Tools))
		for i, tool := range body.Tools {
			oaiTools[i] = OAITool{
				Type: "function",
				Function: OAIToolFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			}
		}
		req.Tools = oaiTools
	}

	// Optional parameters
	if body.Temperature != nil {
		req.Temperature = body.Temperature
	}
	if body.TopP != nil {
		req.TopP = body.TopP
	}
	if len(body.StopSequences) > 0 {
		req.Stop = body.StopSequences
	}
	if body.Stream {
		req.Stream = true
	}

	return req, nil
}

// TranslateResponse converts OpenAI response to Anthropic format
func TranslateResponse(oaiResp *OAIResponse, requestModel string) *AnthropicResponse {
	if len(oaiResp.Choices) == 0 {
		return &AnthropicResponse{
			ID:         getOrDefault(oaiResp.ID, "msg_proxy_error"),
			Type:       "message",
			Role:       "assistant",
			Content:    []ContentBlock{{Type: "text", Text: "No response from model"}},
			Model:      requestModel,
			StopReason: "end_turn",
			Usage:      AnthropicUsage{InputTokens: 0, OutputTokens: 0},
		}
	}

	choice := oaiResp.Choices[0]
	msg := choice.Message
	content := []ContentBlock{}

	hasToolCalls := len(msg.ToolCalls) > 0

	// Add text content if present and no tool calls
	if len(msg.Content) > 0 && !hasToolCalls {
		var contentStr string
		if err := json.Unmarshal(msg.Content, &contentStr); err == nil {
			if contentStr != "" {
				content = append(content, ContentBlock{
					Type: "text",
					Text: contentStr,
				})
			}
		}
	}

	// Add tool calls
	if hasToolCalls {
		for _, tc := range msg.ToolCalls {
			input := json.RawMessage(tc.Function.Arguments)
			// Validate JSON, wrap in raw object if invalid
			var testParse interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &testParse); err != nil {
				rawObj := map[string]string{"raw": tc.Function.Arguments}
				input, _ = json.Marshal(rawObj)
			}

			content = append(content, ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	// Ensure at least one content block
	if len(content) == 0 {
		content = append(content, ContentBlock{Type: "text", Text: ""})
	}

	// Map finish reason
	stopReason := "end_turn"
	switch choice.FinishReason {
	case "tool_calls", "function_call":
		stopReason = "tool_use"
	case "length":
		stopReason = "max_tokens"
	case "stop":
		if hasToolCalls {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}

	// Build usage
	usage := AnthropicUsage{InputTokens: 0, OutputTokens: 0}
	if oaiResp.Usage != nil {
		usage.InputTokens = oaiResp.Usage.PromptTokens
		usage.OutputTokens = oaiResp.Usage.CompletionTokens
	}

	id := oaiResp.ID
	if id == "" {
		id = fmt.Sprintf("msg_%d", time.Now().UnixNano()/1000000)
	}

	return &AnthropicResponse{
		ID:           id,
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        requestModel,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage:        usage,
	}
}

func getOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	// Remove surrounding quotes
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return string(b)
}
