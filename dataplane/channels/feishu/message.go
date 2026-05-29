package feishu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseMessageContent extracts text content from different Feishu message types.
// Supported types: text, post, interactive, image, file, audio, media, sticker.
// For non-text types, returns a descriptive placeholder.
func ParseMessageContent(messageType, rawContent string) (string, error) {
	if rawContent == "" {
		return "", nil
	}

	switch messageType {
	case "text":
		return parseTextContent(rawContent)
	case "post":
		return parsePostContent(rawContent)
	case "interactive":
		return parseInteractiveContent(rawContent)
	case "image":
		return "[image]", nil
	case "file":
		return parseFileContent(rawContent)
	case "audio":
		return "[audio]", nil
	case "media":
		return "[media]", nil
	case "sticker":
		return "[sticker]", nil
	default:
		return fmt.Sprintf("[%s]", messageType), nil
	}
}

// parseTextContent parses a text message.
// Format: {"text": "hello"}
func parseTextContent(raw string) (string, error) {
	var msg struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return "", fmt.Errorf("parse text message: %w", err)
	}
	return msg.Text, nil
}

// parsePostContent parses a rich text (post) message.
// Format: {"title": "...", "content": [[{"tag":"text","text":"..."},{"tag":"at","user_id":"..."}]]}
func parsePostContent(raw string) (string, error) {
	var msg struct {
		Title   string          `json:"title"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return "", fmt.Errorf("parse post message: %w", err)
	}

	var paragraphs [][]struct {
		Tag    string `json:"tag"`
		Text   string `json:"text,omitempty"`
		Href   string `json:"href,omitempty"`
		UserID string `json:"user_id,omitempty"`
	}
	if err := json.Unmarshal(msg.Content, &paragraphs); err != nil {
		// Try direct content format.
		return msg.Title, nil
	}

	var sb strings.Builder
	if msg.Title != "" {
		sb.WriteString(msg.Title)
		sb.WriteString("\n")
	}

	for i, paragraph := range paragraphs {
		if i > 0 {
			sb.WriteString("\n")
		}
		for _, elem := range paragraph {
			switch elem.Tag {
			case "text":
				sb.WriteString(elem.Text)
			case "a":
				sb.WriteString(elem.Text)
			case "at":
				sb.WriteString("@" + elem.UserID)
			}
		}
	}

	return sb.String(), nil
}

// parseInteractiveContent parses an interactive (card action) message.
// Returns the action value if present.
func parseInteractiveContent(raw string) (string, error) {
	var msg struct {
		Action struct {
			Value map[string]string `json:"value"`
			Tag   string           `json:"tag"`
		} `json:"action"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return "[interactive]", nil
	}
	if v, ok := msg.Action.Value["content"]; ok {
		return v, nil
	}
	return "[interactive]", nil
}

// parseFileContent parses a file message and returns a description.
func parseFileContent(raw string) (string, error) {
	var msg struct {
		FileName string `json:"file_name"`
		FileKey  string `json:"file_key"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return "[file]", nil
	}
	if msg.FileName != "" {
		return fmt.Sprintf("[file: %s]", msg.FileName), nil
	}
	return "[file]", nil
}

// extractTextContent extracts text from a V1 event text field.
// V1 text may contain @mentions in format @_user_1.
func extractTextContent(text string) string {
	// Remove @mention patterns for clean text.
	result := text
	// Simple cleanup - remove @_user_N patterns.
	for {
		idx := strings.Index(result, "@_user_")
		if idx == -1 {
			break
		}
		end := idx + 7
		for end < len(result) && result[end] != ' ' && result[end] != '\n' {
			end++
		}
		result = result[:idx] + result[end:]
	}
	return strings.TrimSpace(result)
}

// parseTimestamp parses a Feishu timestamp string (milliseconds since epoch).
func parseTimestamp(ts string) (time.Time, error) {
	ms, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}
