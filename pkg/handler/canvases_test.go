package handler

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockSlackAPI implements provider.SlackAPI for testing canvas handlers.
type mockSlackAPI struct {
	provider.SlackAPI

	getFilesContextFn             func(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error)
	getFileInfoContextFn          func(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error)
	getFileContextFn              func(ctx context.Context, downloadURL string, writer io.Writer) error
	lookupCanvasSectionsContextFn func(ctx context.Context, params slack.LookupCanvasSectionsParams) ([]slack.CanvasSection, error)
	createCanvasContextFn         func(ctx context.Context, title string, documentContent slack.DocumentContent) (string, error)
	editCanvasContextFn           func(ctx context.Context, params slack.EditCanvasParams) error
}

func (m *mockSlackAPI) AuthTest() (*slack.AuthTestResponse, error) {
	return &slack.AuthTestResponse{
		URL:    "https://test.slack.com/",
		Team:   "Test Team",
		User:   "testuser",
		TeamID: "T123",
		UserID: "U123",
	}, nil
}

func (m *mockSlackAPI) AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error) {
	return m.AuthTest()
}

func (m *mockSlackAPI) GetUsersContext(ctx context.Context, options ...slack.GetUsersOption) ([]slack.User, error) {
	return nil, nil
}

func (m *mockSlackAPI) GetUsersInfo(users ...string) (*[]slack.User, error) {
	return &[]slack.User{}, nil
}

func (m *mockSlackAPI) GetConversationsContext(ctx context.Context, params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	return nil, "", nil
}

func (m *mockSlackAPI) GetFilesContext(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error) {
	if m.getFilesContextFn != nil {
		return m.getFilesContextFn(ctx, params)
	}
	return nil, nil, nil
}

func (m *mockSlackAPI) GetFileInfoContext(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
	if m.getFileInfoContextFn != nil {
		return m.getFileInfoContextFn(ctx, fileID, count, page)
	}
	return nil, nil, nil, nil
}

func (m *mockSlackAPI) GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error {
	if m.getFileContextFn != nil {
		return m.getFileContextFn(ctx, downloadURL, writer)
	}
	return nil
}

func (m *mockSlackAPI) LookupCanvasSectionsContext(ctx context.Context, params slack.LookupCanvasSectionsParams) ([]slack.CanvasSection, error) {
	if m.lookupCanvasSectionsContextFn != nil {
		return m.lookupCanvasSectionsContextFn(ctx, params)
	}
	return nil, nil
}

func (m *mockSlackAPI) CreateCanvasContext(ctx context.Context, title string, documentContent slack.DocumentContent) (string, error) {
	if m.createCanvasContextFn != nil {
		return m.createCanvasContextFn(ctx, title, documentContent)
	}
	return "", nil
}

func (m *mockSlackAPI) EditCanvasContext(ctx context.Context, params slack.EditCanvasParams) error {
	if m.editCanvasContextFn != nil {
		return m.editCanvasContextFn(ctx, params)
	}
	return nil
}

func newTestCanvasesHandler(mock *mockSlackAPI) *CanvasesHandler {
	logger := zap.NewNop()
	ap := provider.NewTestProvider(mock, logger)
	return NewCanvasesHandler(ap, logger)
}

func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestCanvasesListHandler(t *testing.T) {
	t.Run("returns CSV of canvases", func(t *testing.T) {
		mock := &mockSlackAPI{
			getFilesContextFn: func(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error) {
				assert.Equal(t, "canvases", params.Types)
				return []slack.File{
					{ID: "F001", Title: "Project Plan", User: "U001"},
					{ID: "F002", Title: "Meeting Notes", User: "U002"},
				}, nil, nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesListHandler(context.Background(), makeRequest(nil))
		require.NoError(t, err)
		require.NotNil(t, result)

		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "F001")
		assert.Contains(t, text, "Project Plan")
		assert.Contains(t, text, "F002")
		assert.Contains(t, text, "Meeting Notes")
	})

	t.Run("returns message when no canvases", func(t *testing.T) {
		mock := &mockSlackAPI{
			getFilesContextFn: func(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error) {
				return []slack.File{}, nil, nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesListHandler(context.Background(), makeRequest(nil))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "No canvases found")
	})

	t.Run("returns error on API failure", func(t *testing.T) {
		mock := &mockSlackAPI{
			getFilesContextFn: func(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error) {
				return nil, nil, errors.New("missing_scope: files:read")
			},
		}

		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesListHandler(context.Background(), makeRequest(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list canvases")
	})
}

func TestCanvasesReadHandler(t *testing.T) {
	t.Run("returns canvas content via download", func(t *testing.T) {
		mock := &mockSlackAPI{
			getFileInfoContextFn: func(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
				assert.Equal(t, "F001", fileID)
				return &slack.File{
					ID:         "F001",
					Title:      "Test Canvas",
					URLPrivate: "https://files.slack.com/canvas/F001",
				}, nil, nil, nil
			},
			getFileContextFn: func(ctx context.Context, downloadURL string, writer io.Writer) error {
				writer.Write([]byte("# Hello World\n\nThis is canvas content."))
				return nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesReadHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id": "F001",
		}))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "Test Canvas")
		assert.Contains(t, text, "Hello World")
	})

	t.Run("requires canvas_id", func(t *testing.T) {
		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesReadHandler(context.Background(), makeRequest(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "canvas_id is required")
	})

	t.Run("returns error for invalid canvas", func(t *testing.T) {
		mock := &mockSlackAPI{
			getFileInfoContextFn: func(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
				return nil, nil, nil, errors.New("file_not_found")
			},
		}

		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesReadHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id": "FINVALID",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read canvas")
	})
}

func TestCanvasesSectionsLookupHandler(t *testing.T) {
	t.Run("returns matching sections", func(t *testing.T) {
		mock := &mockSlackAPI{
			lookupCanvasSectionsContextFn: func(ctx context.Context, params slack.LookupCanvasSectionsParams) ([]slack.CanvasSection, error) {
				assert.Equal(t, "F001", params.CanvasID)
				assert.Equal(t, "important", params.Criteria.ContainsText)
				return []slack.CanvasSection{
					{ID: "temp:C:sec1"},
					{ID: "temp:C:sec2"},
				}, nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesSectionsLookupHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id":     "F001",
			"contains_text": "important",
		}))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "2 section(s)")
		assert.Contains(t, text, "temp:C:sec1")
	})

	t.Run("requires canvas_id", func(t *testing.T) {
		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesSectionsLookupHandler(context.Background(), makeRequest(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "canvas_id is required")
	})
}

func TestCanvasesCreateHandler(t *testing.T) {
	t.Run("creates canvas when enabled", func(t *testing.T) {
		os.Setenv("SLACK_MCP_CANVAS_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{
			createCanvasContextFn: func(ctx context.Context, title string, documentContent slack.DocumentContent) (string, error) {
				assert.Equal(t, "My Canvas", title)
				assert.Equal(t, "markdown", documentContent.Type)
				assert.Equal(t, "# Hello", documentContent.Markdown)
				return "F_NEW_001", nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesCreateHandler(context.Background(), makeRequest(map[string]any{
			"title":   "My Canvas",
			"content": "# Hello",
		}))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "F_NEW_001")
	})

	t.Run("returns error when disabled", func(t *testing.T) {
		os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesCreateHandler(context.Background(), makeRequest(map[string]any{
			"title":   "My Canvas",
			"content": "# Hello",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
		assert.Contains(t, err.Error(), "SLACK_MCP_CANVAS_WRITE_TOOL")
	})
}

func TestCanvasesEditHandler(t *testing.T) {
	t.Run("edits canvas with replace operation", func(t *testing.T) {
		os.Setenv("SLACK_MCP_CANVAS_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{
			editCanvasContextFn: func(ctx context.Context, params slack.EditCanvasParams) error {
				assert.Equal(t, "F001", params.CanvasID)
				require.Len(t, params.Changes, 1)
				assert.Equal(t, "replace", params.Changes[0].Operation)
				assert.Equal(t, "New content", params.Changes[0].DocumentContent.Markdown)
				return nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesEditHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id": "F001",
			"operation": "replace",
			"content":   "New content",
		}))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "edited successfully")
	})

	t.Run("edits canvas with insert_after and section_id", func(t *testing.T) {
		os.Setenv("SLACK_MCP_CANVAS_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{
			editCanvasContextFn: func(ctx context.Context, params slack.EditCanvasParams) error {
				require.Len(t, params.Changes, 1)
				assert.Equal(t, "insert_after", params.Changes[0].Operation)
				assert.Equal(t, "temp:C:sec1", params.Changes[0].SectionID)
				return nil
			},
		}

		h := newTestCanvasesHandler(mock)
		result, err := h.CanvasesEditHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id":  "F001",
			"operation":  "insert_after",
			"section_id": "temp:C:sec1",
			"content":    "Inserted content",
		}))
		require.NoError(t, err)
		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "edited successfully")
	})

	t.Run("returns error when disabled", func(t *testing.T) {
		os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesEditHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id": "F001",
			"operation": "replace",
			"content":   "New content",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("requires canvas_id", func(t *testing.T) {
		os.Setenv("SLACK_MCP_CANVAS_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesEditHandler(context.Background(), makeRequest(map[string]any{
			"operation": "replace",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "canvas_id is required")
	})

	t.Run("requires operation", func(t *testing.T) {
		os.Setenv("SLACK_MCP_CANVAS_WRITE_TOOL", "true")
		defer os.Unsetenv("SLACK_MCP_CANVAS_WRITE_TOOL")

		mock := &mockSlackAPI{}
		h := newTestCanvasesHandler(mock)
		_, err := h.CanvasesEditHandler(context.Background(), makeRequest(map[string]any{
			"canvas_id": "F001",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "operation is required")
	})
}

// Ensure tests don't leave env vars set
func init() {
	_ = strings.NewReader("") // ensure strings is used
}
