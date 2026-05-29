package feishu

import "encoding/json"

// CardMessage represents an interactive card message to send back to Feishu.
type CardMessage struct {
	Header   *CardHeader   `json:"header,omitempty"`
	Elements []CardElement `json:"elements,omitempty"`
}

// CardHeader is the card title section.
type CardHeader struct {
	Title    *CardText `json:"title"`
	Template string    `json:"template,omitempty"` // color: blue, green, red, etc.
}

// CardText is a text element.
type CardText struct {
	Tag     string `json:"tag"`     // "plain_text" or "lark_md"
	Content string `json:"content"`
}

// CardElement is a generic card element (div, action, hr, etc.).
type CardElement struct {
	Tag     string        `json:"tag"`
	Text    *CardText     `json:"text,omitempty"`
	Actions []CardAction  `json:"actions,omitempty"`
	Fields  []CardField   `json:"fields,omitempty"`
	Content string        `json:"content,omitempty"` // for markdown div
}

// CardAction is a button or other interactive element.
type CardAction struct {
	Tag   string            `json:"tag"`
	Text  *CardText         `json:"text"`
	Type  string            `json:"type,omitempty"` // "primary", "danger", "default"
	Value map[string]string `json:"value,omitempty"`
	URL   string            `json:"url,omitempty"`
}

// CardField is a field in a multi-column layout.
type CardField struct {
	IsShort bool      `json:"is_short"`
	Text    *CardText `json:"text"`
}

// NewCardMessage creates a new card message with a title.
func NewCardMessage(title string) *CardMessage {
	return &CardMessage{
		Header: &CardHeader{
			Title: &CardText{
				Tag:     "plain_text",
				Content: title,
			},
		},
	}
}

// SetTemplate sets the header color template.
func (c *CardMessage) SetTemplate(template string) *CardMessage {
	if c.Header != nil {
		c.Header.Template = template
	}
	return c
}

// AddMarkdown adds a markdown content element.
func (c *CardMessage) AddMarkdown(content string) *CardMessage {
	c.Elements = append(c.Elements, CardElement{
		Tag: "div",
		Text: &CardText{
			Tag:     "lark_md",
			Content: content,
		},
	})
	return c
}

// AddPlainText adds a plain text element.
func (c *CardMessage) AddPlainText(content string) *CardMessage {
	c.Elements = append(c.Elements, CardElement{
		Tag: "div",
		Text: &CardText{
			Tag:     "plain_text",
			Content: content,
		},
	})
	return c
}

// AddHR adds a horizontal rule.
func (c *CardMessage) AddHR() *CardMessage {
	c.Elements = append(c.Elements, CardElement{
		Tag: "hr",
	})
	return c
}

// AddButton adds a button action element.
func (c *CardMessage) AddButton(text, buttonType string, value map[string]string) *CardMessage {
	action := CardElement{
		Tag: "action",
		Actions: []CardAction{
			{
				Tag:   "button",
				Text:  &CardText{Tag: "plain_text", Content: text},
				Type:  buttonType,
				Value: value,
			},
		},
	}
	c.Elements = append(c.Elements, action)
	return c
}

// ToFeishuResponse converts the card to the Feishu webhook response format.
// This is used for immediate card reply in event callback response body.
func (c *CardMessage) ToFeishuResponse() map[string]interface{} {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
	}

	if c.Header != nil {
		card["header"] = c.Header
	}
	if len(c.Elements) > 0 {
		card["elements"] = c.Elements
	}

	return map[string]interface{}{
		"msg_type": "interactive",
		"card":     card,
	}
}

// ToJSON serializes the card to JSON bytes (for API-based sending).
func (c *CardMessage) ToJSON() ([]byte, error) {
	return json.Marshal(c.ToFeishuResponse())
}
