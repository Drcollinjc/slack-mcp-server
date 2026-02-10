package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/provider/lists"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type ListsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewListsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ListsHandler {
	return &ListsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// ListRow represents a row in the CSV output for list items.
type ListRow struct {
	RecordID string `csv:"RecordID"`
	// Dynamic columns are handled by building CSV manually
}

// ListsGetItemsHandler retrieves items from a list with pagination, formatting as CSV.
func (lh *ListsHandler) ListsGetItemsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lh.logger.Debug("ListsGetItemsHandler called")

	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	listID := request.GetString("list_id", "")
	if listID == "" {
		return nil, errors.New("list_id is required")
	}
	cursor := request.GetString("cursor", "")
	limit := request.GetInt("limit", 100)

	listsClient := lh.apiProvider.Lists()
	if listsClient == nil {
		return nil, errors.New("lists client is not available")
	}

	resp, err := listsClient.GetItems(ctx, listID, cursor, limit)
	if err != nil {
		lh.logger.Error("Failed to get list items", zap.String("list_id", listID), zap.Error(err))
		return nil, fmt.Errorf("failed to get items from list %s: %w", listID, err)
	}

	if len(resp.Items) == 0 {
		return mcp.NewToolResultText("No items found."), nil
	}

	csv := lh.formatItemsCSV(resp.Items)

	// Append cursor info if available
	nextCursor := resp.ResponseMetadata.NextCursor
	if nextCursor != "" {
		csv += fmt.Sprintf("\n# Next cursor: %s", nextCursor)
	}

	return mcp.NewToolResultText(csv), nil
}

// ListsGetItemHandler retrieves a single item from a list.
func (lh *ListsHandler) ListsGetItemHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lh.logger.Debug("ListsGetItemHandler called")

	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	listID := request.GetString("list_id", "")
	if listID == "" {
		return nil, errors.New("list_id is required")
	}
	recordID := request.GetString("record_id", "")
	if recordID == "" {
		return nil, errors.New("record_id is required")
	}

	listsClient := lh.apiProvider.Lists()
	if listsClient == nil {
		return nil, errors.New("lists client is not available")
	}

	resp, err := listsClient.GetItem(ctx, listID, recordID)
	if err != nil {
		lh.logger.Error("Failed to get list item",
			zap.String("list_id", listID),
			zap.String("record_id", recordID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get item %s from list %s: %w", recordID, listID, err)
	}

	result := lh.formatSingleItem(resp.Record, resp.List.ListMetadata.Schema)
	return mcp.NewToolResultText(result), nil
}

// isListWriteEnabled checks if list write tools are enabled via env var.
func isListWriteEnabled() bool {
	v := strings.ToLower(os.Getenv("SLACK_MCP_LIST_WRITE_TOOL"))
	return v == "true" || v == "1" || v == "yes"
}

// ListsAddItemHandler creates a new item in a list.
func (lh *ListsHandler) ListsAddItemHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lh.logger.Debug("ListsAddItemHandler called")

	if !isListWriteEnabled() {
		return nil, errors.New(
			"list write tools are disabled by default. " +
				"To enable them, set the SLACK_MCP_LIST_WRITE_TOOL environment variable to 'true'")
	}

	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	listID := request.GetString("list_id", "")
	if listID == "" {
		return nil, errors.New("list_id is required")
	}

	fieldsStr := request.GetString("fields", "")
	if fieldsStr == "" {
		return nil, errors.New("fields is required (JSON object mapping column IDs to values)")
	}

	var rawFields map[string]any
	if err := json.Unmarshal([]byte(fieldsStr), &rawFields); err != nil {
		return nil, fmt.Errorf("fields must be valid JSON: %w", err)
	}

	// Convert fields: auto-wrap string values as rich_text for text columns
	fields := make(map[string]json.RawMessage)
	for k, v := range rawFields {
		switch val := v.(type) {
		case string:
			// Auto-wrap plain strings as rich_text
			fields[k] = lists.WrapTextAsRichText(val)
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal field %s: %w", k, err)
			}
			fields[k] = data
		}
	}

	listsClient := lh.apiProvider.Lists()
	if listsClient == nil {
		return nil, errors.New("lists client is not available")
	}

	resp, err := listsClient.AddItem(ctx, listID, fields)
	if err != nil {
		lh.logger.Error("Failed to add list item", zap.String("list_id", listID), zap.Error(err))
		return nil, fmt.Errorf("failed to add item to list %s: %w", listID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Item created successfully. Record ID: %s", resp.Item.ID)), nil
}

// ListsUpdateItemHandler updates a specific field in a list item.
func (lh *ListsHandler) ListsUpdateItemHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lh.logger.Debug("ListsUpdateItemHandler called")

	if !isListWriteEnabled() {
		return nil, errors.New(
			"list write tools are disabled by default. " +
				"To enable them, set the SLACK_MCP_LIST_WRITE_TOOL environment variable to 'true'")
	}

	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	listID := request.GetString("list_id", "")
	if listID == "" {
		return nil, errors.New("list_id is required")
	}
	recordID := request.GetString("record_id", "")
	if recordID == "" {
		return nil, errors.New("record_id is required")
	}
	columnID := request.GetString("column_id", "")
	if columnID == "" {
		return nil, errors.New("column_id is required")
	}
	value := request.GetString("value", "")

	// Auto-wrap string values as rich_text
	fields := map[string]json.RawMessage{
		columnID: lists.WrapTextAsRichText(value),
	}

	listsClient := lh.apiProvider.Lists()
	if listsClient == nil {
		return nil, errors.New("lists client is not available")
	}

	_, err := listsClient.UpdateItem(ctx, listID, recordID, fields)
	if err != nil {
		lh.logger.Error("Failed to update list item",
			zap.String("list_id", listID),
			zap.String("record_id", recordID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to update item %s in list %s: %w", recordID, listID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Item %s updated successfully.", recordID)), nil
}

// ListsDeleteItemHandler removes an item from a list.
func (lh *ListsHandler) ListsDeleteItemHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lh.logger.Debug("ListsDeleteItemHandler called")

	if !isListWriteEnabled() {
		return nil, errors.New(
			"list write tools are disabled by default. " +
				"To enable them, set the SLACK_MCP_LIST_WRITE_TOOL environment variable to 'true'")
	}

	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	listID := request.GetString("list_id", "")
	if listID == "" {
		return nil, errors.New("list_id is required")
	}
	recordID := request.GetString("record_id", "")
	if recordID == "" {
		return nil, errors.New("record_id is required")
	}

	listsClient := lh.apiProvider.Lists()
	if listsClient == nil {
		return nil, errors.New("lists client is not available")
	}

	err := listsClient.DeleteItem(ctx, listID, recordID)
	if err != nil {
		lh.logger.Error("Failed to delete list item",
			zap.String("list_id", listID),
			zap.String("record_id", recordID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to delete item %s from list %s: %w", recordID, listID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Item %s deleted successfully.", recordID)), nil
}

// formatItemsCSV formats list items as CSV. Since items.list doesn't return schema,
// columns are derived from the fields present in items and keyed by field key.
func (lh *ListsHandler) formatItemsCSV(items []lists.Item) string {
	usersMap := lh.apiProvider.ProvideUsersMap().Users

	// Collect all unique columns across all items, preserving order of first appearance
	type colInfo struct {
		columnID string
		key      string
	}
	seen := make(map[string]bool)
	var columns []colInfo
	for _, item := range items {
		for colID, field := range item.Fields {
			if !seen[colID] {
				seen[colID] = true
				columns = append(columns, colInfo{columnID: colID, key: field.Key})
			}
		}
	}

	// Build header — use field key as the header name
	headers := []string{"RecordID"}
	for _, col := range columns {
		headers = append(headers, col.key)
	}

	// Build rows
	type csvRow struct {
		fields []string
	}
	var rows []csvRow
	for _, item := range items {
		row := csvRow{fields: []string{item.ID}}
		for _, col := range columns {
			field, ok := item.Fields[col.columnID]
			if !ok {
				row.fields = append(row.fields, "")
				continue
			}
			row.fields = append(row.fields, formatFieldValueAuto(field, usersMap))
		}
		rows = append(rows, row)
	}

	// Build CSV string manually since columns are dynamic
	var sb strings.Builder
	sb.WriteString(strings.Join(headers, ",") + "\n")
	for _, row := range rows {
		escaped := make([]string, len(row.fields))
		for i, f := range row.fields {
			escaped[i] = csvEscape(f)
		}
		sb.WriteString(strings.Join(escaped, ",") + "\n")
	}

	return sb.String()
}

// formatSingleItem formats a single list item as key-value text using schema columns.
func (lh *ListsHandler) formatSingleItem(item lists.Item, schema []lists.Column) string {
	usersMap := lh.apiProvider.ProvideUsersMap().Users

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Record ID: %s\n\n", item.ID))
	for _, col := range schema {
		field, ok := item.Fields[col.ID]
		if !ok {
			sb.WriteString(fmt.Sprintf("%s: (empty)\n", col.Name))
			continue
		}
		val := formatFieldValue(field, col.Type, usersMap)
		sb.WriteString(fmt.Sprintf("%s: %s\n", col.Name, val))
	}

	return sb.String()
}

// formatFieldValue converts an ItemField to human-readable text based on column type.
func formatFieldValue(field lists.ItemField, colType string, usersMap map[string]slack.User) string {
	switch colType {
	case "text":
		// Use the rich_text sub-field if available, fallback to text
		if len(field.RichText) > 0 {
			return extractTextFromRichText(field.RichText)
		}
		if field.Text != "" {
			return field.Text
		}
		return extractStringValue(field.Value)
	case "todo_assignee", "user":
		if len(field.User) > 0 {
			return formatUserField(field.User, usersMap)
		}
		return formatUserField(field.Value, usersMap)
	case "todo_due_date", "date":
		if len(field.Date) > 0 {
			return formatDateField(field.Date)
		}
		return formatDateField(field.Value)
	case "select":
		if len(field.Select) > 0 {
			return formatSelectField(field.Select)
		}
		return formatSelectField(field.Value)
	case "todo_completed", "checkbox":
		if field.Checkbox != nil {
			if *field.Checkbox {
				return "Yes"
			}
			return "No"
		}
		return formatCheckboxField(field.Value)
	case "number":
		return formatNumberField(field.Value)
	default:
		if field.Text != "" {
			return field.Text
		}
		return extractStringValue(field.Value)
	}
}

// formatFieldValueAuto infers the field type from available sub-fields (for items.list which has no schema).
func formatFieldValueAuto(field lists.ItemField, usersMap map[string]slack.User) string {
	// Text fields have rich_text or text
	if len(field.RichText) > 0 {
		return extractTextFromRichText(field.RichText)
	}
	if field.Text != "" {
		return field.Text
	}
	// Checkbox fields
	if field.Checkbox != nil {
		if *field.Checkbox {
			return "Yes"
		}
		return "No"
	}
	// User fields
	if len(field.User) > 0 {
		return formatUserField(field.User, usersMap)
	}
	// Date fields
	if len(field.Date) > 0 {
		return formatDateField(field.Date)
	}
	// Select fields
	if len(field.Select) > 0 {
		return formatSelectField(field.Select)
	}
	// Fallback to raw value
	return extractStringValue(field.Value)
}

// extractStringValue extracts a string from a json.RawMessage.
func extractStringValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// extractTextFromRichText pulls plain text out of a rich_text field.
func extractTextFromRichText(raw json.RawMessage) string {
	// Try as rich_text blocks array
	var blocks []lists.RichTextBlock
	if json.Unmarshal(raw, &blocks) == nil && len(blocks) > 0 {
		var parts []string
		for _, block := range blocks {
			for _, elem := range block.Elements {
				for _, run := range elem.Elements {
					if run.Text != "" {
						parts = append(parts, run.Text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	}
	// Try as plain string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// formatUserField resolves user IDs to display names.
func formatUserField(raw json.RawMessage, usersMap map[string]slack.User) string {
	// Could be a single user ID string or array
	var userID string
	if json.Unmarshal(raw, &userID) == nil {
		if u, ok := usersMap[userID]; ok {
			return u.RealName
		}
		return userID
	}
	var userIDs []string
	if json.Unmarshal(raw, &userIDs) == nil {
		var names []string
		for _, uid := range userIDs {
			if u, ok := usersMap[uid]; ok {
				names = append(names, u.RealName)
			} else {
				names = append(names, uid)
			}
		}
		return strings.Join(names, ", ")
	}
	return string(raw)
}

// formatDateField formats date values.
func formatDateField(raw json.RawMessage) string {
	// Try as array of date strings (API format)
	var dates []string
	if json.Unmarshal(raw, &dates) == nil && len(dates) > 0 {
		return strings.Join(dates, ", ")
	}
	// Try as unix timestamp
	var ts float64
	if json.Unmarshal(raw, &ts) == nil && ts > 0 {
		return time.Unix(int64(ts), 0).Format("2006-01-02")
	}
	// Try as string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// formatSelectField formats select/dropdown values.
func formatSelectField(raw json.RawMessage) string {
	// Try as array of option IDs (API format)
	var opts []string
	if json.Unmarshal(raw, &opts) == nil && len(opts) > 0 {
		return strings.Join(opts, ", ")
	}
	// Try as single string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// formatCheckboxField formats checkbox values.
func formatCheckboxField(raw json.RawMessage) string {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		if b {
			return "Yes"
		}
		return "No"
	}
	return string(raw)
}

// formatNumberField formats number values.
func formatNumberField(raw json.RawMessage) string {
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return fmt.Sprintf("%g", n)
	}
	return string(raw)
}

// csvEscape wraps a value in quotes if it contains commas, newlines, or quotes.
func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n\r") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// ensure gocsv is imported (used via formatItemsCSV build)
var _ = gocsv.MarshalBytes
