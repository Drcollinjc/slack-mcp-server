package lists

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultAPIURL = "https://slack.com/api/"

// Client is an HTTP client for the Slack Lists API (slackLists.* endpoints).
type Client struct {
	token      string
	httpClient *http.Client
	apiURL     string
}

// NewClient creates a new Lists API client.
func NewClient(token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		token:      token,
		httpClient: httpClient,
		apiURL:     defaultAPIURL,
	}
}

// postJSON sends a POST request with a JSON body to the given Slack API method.
func (c *Client) postJSON(ctx context.Context, method string, payload any) ([]byte, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request for %s: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+method, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call %s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", method, err)
	}

	return body, nil
}

// post sends a POST request to the given Slack API method with form-encoded parameters.
func (c *Client) post(ctx context.Context, method string, params url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+method, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call %s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", method, err)
	}

	return body, nil
}

// GetItems retrieves items from a list with optional pagination.
func (c *Client) GetItems(ctx context.Context, listID string, cursor string, limit int) (*GetItemsResponse, error) {
	params := url.Values{
		"list_id": {listID},
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	body, err := c.post(ctx, "slackLists.items.list", params)
	if err != nil {
		return nil, err
	}

	var resp GetItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse slackLists.items.list response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("slackLists.items.list failed: %s", resp.Error)
	}

	return &resp, nil
}

// GetItem retrieves a single item from a list by record ID.
func (c *Client) GetItem(ctx context.Context, listID string, recordID string) (*GetItemResponse, error) {
	params := url.Values{
		"list_id": {listID},
		"id":      {recordID},
	}

	body, err := c.post(ctx, "slackLists.items.info", params)
	if err != nil {
		return nil, err
	}

	var resp GetItemResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse slackLists.items.info response: %w\nBody: %s", err, string(body))
	}
	if !resp.OK {
		return nil, fmt.Errorf("slackLists.items.info failed: %s", resp.Error)
	}

	return &resp, nil
}

// AddItem creates a new item in a list with the given field values.
// Fields is a map of column_id to the typed value (already wrapped as rich_text, etc.).
func (c *Client) AddItem(ctx context.Context, listID string, fields map[string]json.RawMessage) (*AddItemResponse, error) {
	// Convert flat fields map to initial_fields array format:
	// [{"column_id": "ColXXX", "rich_text": [...]}]
	var initialFields []json.RawMessage
	for colID, val := range fields {
		fieldObj := map[string]json.RawMessage{
			"column_id": json.RawMessage(fmt.Sprintf("%q", colID)),
			"rich_text": val,
		}
		data, err := json.Marshal(fieldObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal field %s: %w", colID, err)
		}
		initialFields = append(initialFields, data)
	}

	payload := map[string]any{
		"list_id":        listID,
		"initial_fields": initialFields,
	}

	body, err := c.postJSON(ctx, "slackLists.items.create", payload)
	if err != nil {
		return nil, err
	}

	var resp AddItemResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse slackLists.items.create response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("slackLists.items.create failed: %s", resp.Error)
	}

	return &resp, nil
}

// UpdateItem updates a specific field in a list item.
// Fields is a map of column_id to the typed value (already wrapped as rich_text, etc.).
func (c *Client) UpdateItem(ctx context.Context, listID string, recordID string, fields map[string]json.RawMessage) (*UpdateItemResponse, error) {
	// Convert to cells array format:
	// [{"row_id": "RecXXX", "column_id": "ColXXX", "rich_text": [...]}]
	var cells []json.RawMessage
	for colID, val := range fields {
		cellObj := map[string]json.RawMessage{
			"row_id":    json.RawMessage(fmt.Sprintf("%q", recordID)),
			"column_id": json.RawMessage(fmt.Sprintf("%q", colID)),
			"rich_text": val,
		}
		data, err := json.Marshal(cellObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal cell for column %s: %w", colID, err)
		}
		cells = append(cells, data)
	}

	payload := map[string]any{
		"list_id": listID,
		"cells":   cells,
	}

	body, err := c.postJSON(ctx, "slackLists.items.update", payload)
	if err != nil {
		return nil, err
	}

	var resp UpdateItemResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse slackLists.items.update response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("slackLists.items.update failed: %s", resp.Error)
	}

	return &resp, nil
}

// DeleteItem removes an item from a list.
func (c *Client) DeleteItem(ctx context.Context, listID string, recordID string) error {
	params := url.Values{
		"list_id": {listID},
		"id":      {recordID},
	}

	body, err := c.post(ctx, "slackLists.items.delete", params)
	if err != nil {
		return err
	}

	var resp DeleteItemResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse slackLists.items.delete response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("slackLists.items.delete failed: %s", resp.Error)
	}

	return nil
}
