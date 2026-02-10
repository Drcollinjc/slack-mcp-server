package handler

import (
	"context"
	"os"
	"testing"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/provider/lists"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestListsHandler(listsClient *lists.Client) *ListsHandler {
	logger := zap.NewNop()
	mock := &mockSlackAPI{}
	ap := provider.NewTestProviderWithLists(mock, listsClient, logger)
	return NewListsHandler(ap, logger)
}

func TestListsGetItemsHandler(t *testing.T) {
	t.Run("requires list_id", func(t *testing.T) {
		h := newTestListsHandler(nil)
		_, err := h.ListsGetItemsHandler(context.Background(), makeRequest(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list_id is required")
	})

	t.Run("returns error when lists client unavailable", func(t *testing.T) {
		h := newTestListsHandler(nil)
		_, err := h.ListsGetItemsHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lists client is not available")
	})
}

func TestListsGetItemHandler(t *testing.T) {
	t.Run("requires list_id", func(t *testing.T) {
		h := newTestListsHandler(nil)
		_, err := h.ListsGetItemHandler(context.Background(), makeRequest(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list_id is required")
	})

	t.Run("requires record_id", func(t *testing.T) {
		h := newTestListsHandler(nil)
		_, err := h.ListsGetItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "record_id is required")
	})
}

func TestListsAddItemHandler(t *testing.T) {
	t.Run("returns error when disabled", func(t *testing.T) {
		os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")
		h := newTestListsHandler(nil)
		_, err := h.ListsAddItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
			"fields":  `{"Col001": "test"}`,
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
		assert.Contains(t, err.Error(), "SLACK_MCP_LIST_WRITE_TOOL")
	})

	t.Run("requires list_id when enabled", func(t *testing.T) {
		os.Setenv("SLACK_MCP_LIST_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")

		h := newTestListsHandler(nil)
		_, err := h.ListsAddItemHandler(context.Background(), makeRequest(map[string]any{
			"fields": `{"Col001": "test"}`,
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list_id is required")
	})

	t.Run("requires fields when enabled", func(t *testing.T) {
		os.Setenv("SLACK_MCP_LIST_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")

		h := newTestListsHandler(nil)
		_, err := h.ListsAddItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fields is required")
	})
}

func TestListsUpdateItemHandler(t *testing.T) {
	t.Run("returns error when disabled", func(t *testing.T) {
		os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")
		h := newTestListsHandler(nil)
		_, err := h.ListsUpdateItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id":   "F001",
			"record_id": "Rec001",
			"column_id": "Col001",
			"value":     "updated",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("requires all params when enabled", func(t *testing.T) {
		os.Setenv("SLACK_MCP_LIST_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")

		h := newTestListsHandler(nil)

		_, err := h.ListsUpdateItemHandler(context.Background(), makeRequest(map[string]any{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list_id is required")

		_, err = h.ListsUpdateItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "record_id is required")

		_, err = h.ListsUpdateItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id":   "F001",
			"record_id": "Rec001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "column_id is required")
	})
}

func TestListsDeleteItemHandler(t *testing.T) {
	t.Run("returns error when disabled", func(t *testing.T) {
		os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")
		h := newTestListsHandler(nil)
		_, err := h.ListsDeleteItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id":   "F001",
			"record_id": "Rec001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("requires list_id and record_id when enabled", func(t *testing.T) {
		os.Setenv("SLACK_MCP_LIST_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_LIST_WRITE_TOOL")

		h := newTestListsHandler(nil)
		_, err := h.ListsDeleteItemHandler(context.Background(), makeRequest(map[string]any{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list_id is required")

		_, err = h.ListsDeleteItemHandler(context.Background(), makeRequest(map[string]any{
			"list_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "record_id is required")
	})
}

func TestFormatFieldValue(t *testing.T) {
	t.Run("formats rich_text", func(t *testing.T) {
		raw := []byte(`[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[{"type":"text","text":"Hello World"}]}]}]`)
		result := extractTextFromRichText(raw)
		assert.Equal(t, "Hello World", result)
	})

	t.Run("formats plain string", func(t *testing.T) {
		raw := []byte(`"simple text"`)
		result := extractTextFromRichText(raw)
		assert.Equal(t, "simple text", result)
	})

	t.Run("formats checkbox true", func(t *testing.T) {
		result := formatCheckboxField([]byte(`true`))
		assert.Equal(t, "Yes", result)
	})

	t.Run("formats checkbox false", func(t *testing.T) {
		result := formatCheckboxField([]byte(`false`))
		assert.Equal(t, "No", result)
	})

	t.Run("formats number", func(t *testing.T) {
		result := formatNumberField([]byte(`42.5`))
		assert.Equal(t, "42.5", result)
	})

	t.Run("formats date timestamp", func(t *testing.T) {
		result := formatDateField([]byte(`1704067200`))
		assert.Contains(t, result, "2024-01-01")
	})

	t.Run("formats select", func(t *testing.T) {
		result := formatSelectField([]byte(`"high"`))
		assert.Equal(t, "high", result)
	})
}

func TestWrapTextAsRichText(t *testing.T) {
	raw := lists.WrapTextAsRichText("Hello")
	assert.Contains(t, string(raw), "rich_text")
	assert.Contains(t, string(raw), "Hello")
	assert.Contains(t, string(raw), "rich_text_section")
}

func TestCsvEscape(t *testing.T) {
	assert.Equal(t, "simple", csvEscape("simple"))
	assert.Equal(t, "\"has,comma\"", csvEscape("has,comma"))
	assert.Equal(t, "\"has\"\"quote\"", csvEscape("has\"quote"))
	assert.Equal(t, "\"has\nnewline\"", csvEscape("has\nnewline"))
}
