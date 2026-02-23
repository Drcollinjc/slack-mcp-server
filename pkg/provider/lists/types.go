package lists

import "encoding/json"

// SlackResponse is the base response from Slack API calls.
type SlackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Column represents a list column definition from the schema.
type Column struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Key     string          `json:"key"`
	Type    string          `json:"type"`
	Options json.RawMessage `json:"options,omitempty"`
}

// RichTextBlock represents a Block Kit rich_text block.
type RichTextBlock struct {
	Type     string            `json:"type"`
	Elements []RichTextElement `json:"elements"`
}

// RichTextElement represents an element within a rich_text block.
type RichTextElement struct {
	Type     string        `json:"type"`
	Elements []RichTextRun `json:"elements,omitempty"`
}

// RichTextRun represents a text run within a rich_text element.
type RichTextRun struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ItemField represents a single field value in a list item as returned by the API.
type ItemField struct {
	Key      string          `json:"key"`
	ColumnID string          `json:"column_id"`
	Value    json.RawMessage `json:"value"`
	Text     string          `json:"text,omitempty"`
	RichText json.RawMessage `json:"rich_text,omitempty"`
	Checkbox *bool           `json:"checkbox,omitempty"`
	User     json.RawMessage `json:"user,omitempty"`
	Date     json.RawMessage `json:"date,omitempty"`
	Select   json.RawMessage `json:"select,omitempty"`
}

// Item represents a list item (record).
type Item struct {
	ID     string               `json:"id"`
	Fields map[string]ItemField `json:"-"` // keyed by column_id, populated by UnmarshalJSON
}

// UnmarshalJSON converts the API's fields array into a map keyed by column_id.
func (item *Item) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID     string      `json:"id"`
		Fields []ItemField `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	item.ID = raw.ID
	item.Fields = make(map[string]ItemField, len(raw.Fields))
	for _, f := range raw.Fields {
		item.Fields[f.ColumnID] = f
	}
	return nil
}

// ListMetadata contains the schema and views for a list.
type ListMetadata struct {
	Schema []Column `json:"schema"`
}

// ListInfo contains basic list information returned by items.info.
type ListInfo struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	ListMetadata ListMetadata `json:"list_metadata"`
}

// GetItemsResponse is the response from slackLists.items.list.
type GetItemsResponse struct {
	SlackResponse
	Items            []Item `json:"items"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor,omitempty"`
	} `json:"response_metadata,omitempty"`
}

// GetItemResponse is the response from slackLists.items.info.
type GetItemResponse struct {
	SlackResponse
	List   ListInfo `json:"list"`
	Record Item     `json:"record"`
}

// AddItemResponse is the response from slackLists.items.create.
type AddItemResponse struct {
	SlackResponse
	Item Item `json:"item"`
}

// UpdateItemResponse is the response from slackLists.items.update.
type UpdateItemResponse struct {
	SlackResponse
}

// DeleteItemResponse is the response from slackLists.items.delete.
type DeleteItemResponse struct {
	SlackResponse
}

// WrapTextAsRichText converts a plain string into Block Kit rich_text JSON
// that the Slack Lists API requires for text-type columns.
func WrapTextAsRichText(text string) json.RawMessage {
	block := RichTextBlock{
		Type: "rich_text",
		Elements: []RichTextElement{
			{
				Type: "rich_text_section",
				Elements: []RichTextRun{
					{
						Type: "text",
						Text: text,
					},
				},
			},
		},
	}
	data, _ := json.Marshal([]RichTextBlock{block})
	return data
}
