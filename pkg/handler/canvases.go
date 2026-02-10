package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type CanvasItem struct {
	ID        string `csv:"ID"`
	Title     string `csv:"Title"`
	CreatedBy string `csv:"CreatedBy"`
	Updated   string `csv:"Updated"`
}

type CanvasesHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewCanvasesHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *CanvasesHandler {
	return &CanvasesHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// CanvasesListHandler lists canvases in the workspace using files.list with types=canvas.
func (ch *CanvasesHandler) CanvasesListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CanvasesListHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	channelID := request.GetString("channel_id", "")
	limit := request.GetInt("limit", 100)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	params := slack.GetFilesParameters{
		Types: "canvases",
		Count: limit,
	}
	if channelID != "" {
		params.Channel = channelID
	}

	files, _, err := ch.apiProvider.Slack().GetFilesContext(ctx, params)
	if err != nil {
		ch.logger.Error("Failed to list canvases", zap.Error(err))
		return nil, fmt.Errorf("failed to list canvases: %w", err)
	}

	if len(files) == 0 {
		return mcp.NewToolResultText("No canvases found."), nil
	}

	usersMap := ch.apiProvider.ProvideUsersMap().Users
	var items []CanvasItem
	for _, f := range files {
		createdBy := f.User
		if u, ok := usersMap[f.User]; ok {
			createdBy = u.RealName
		}
		updated := time.Unix(int64(f.Timestamp.Time().Unix()), 0).Format(time.RFC3339)
		items = append(items, CanvasItem{
			ID:        f.ID,
			Title:     f.Title,
			CreatedBy: createdBy,
			Updated:   updated,
		})
	}

	csvBytes, err := gocsv.MarshalBytes(&items)
	if err != nil {
		ch.logger.Error("Failed to marshal canvases to CSV", zap.Error(err))
		return nil, fmt.Errorf("failed to format canvases: %w", err)
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// CanvasesReadHandler retrieves canvas content as markdown using files.info.
func (ch *CanvasesHandler) CanvasesReadHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CanvasesReadHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	canvasID := request.GetString("canvas_id", "")
	if canvasID == "" {
		return nil, errors.New("canvas_id is required")
	}

	file, _, _, err := ch.apiProvider.Slack().GetFileInfoContext(ctx, canvasID, 0, 0)
	if err != nil {
		ch.logger.Error("Failed to get canvas info", zap.String("canvas_id", canvasID), zap.Error(err))
		return nil, fmt.Errorf("failed to read canvas %s: %w", canvasID, err)
	}

	// Try to get content via the file's preview or download URL
	if file.URLPrivate != "" {
		var buf bytes.Buffer
		err := ch.apiProvider.Slack().GetFileContext(ctx, file.URLPrivate, &buf)
		if err != nil {
			ch.logger.Warn("Failed to download canvas content, falling back to preview",
				zap.String("canvas_id", canvasID), zap.Error(err))
		} else if buf.Len() > 0 {
			result := fmt.Sprintf("# %s\n\n%s", file.Title, buf.String())
			return mcp.NewToolResultText(result), nil
		}
	}

	// Fallback: use preview content if available
	if file.Preview != "" {
		result := fmt.Sprintf("# %s\n\n%s", file.Title, file.Preview)
		return mcp.NewToolResultText(result), nil
	}

	// Last resort: return metadata only
	return mcp.NewToolResultText(fmt.Sprintf("# %s\n\nCanvas content could not be retrieved. Canvas ID: %s", file.Title, canvasID)), nil
}

// CanvasesSectionsLookupHandler finds sections within a canvas matching criteria.
func (ch *CanvasesHandler) CanvasesSectionsLookupHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CanvasesSectionsLookupHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	canvasID := request.GetString("canvas_id", "")
	if canvasID == "" {
		return nil, errors.New("canvas_id is required")
	}

	containsText := request.GetString("contains_text", "")
	sectionTypes := request.GetString("section_types", "")

	criteria := slack.LookupCanvasSectionsCriteria{}
	if containsText != "" {
		criteria.ContainsText = containsText
	}
	if sectionTypes != "" {
		criteria.SectionTypes = strings.Split(sectionTypes, ",")
		for i, st := range criteria.SectionTypes {
			criteria.SectionTypes[i] = strings.TrimSpace(st)
		}
	}

	params := slack.LookupCanvasSectionsParams{
		CanvasID: canvasID,
		Criteria: criteria,
	}

	sections, err := ch.apiProvider.Slack().LookupCanvasSectionsContext(ctx, params)
	if err != nil {
		ch.logger.Error("Failed to lookup canvas sections",
			zap.String("canvas_id", canvasID), zap.Error(err))
		return nil, fmt.Errorf("failed to lookup sections in canvas %s: %w", canvasID, err)
	}

	if len(sections) == 0 {
		return mcp.NewToolResultText("No matching sections found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d section(s):\n\n", len(sections)))
	for _, s := range sections {
		sb.WriteString(fmt.Sprintf("- Section ID: %s\n", s.ID))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// isCanvasWriteEnabled checks if canvas write tools are enabled via env var.
func isCanvasWriteEnabled() bool {
	v := strings.ToLower(os.Getenv("SLACK_MCP_CANVAS_WRITE_TOOL"))
	return v == "true" || v == "1" || v == "yes"
}

// CanvasesCreateHandler creates a new canvas with markdown content.
func (ch *CanvasesHandler) CanvasesCreateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CanvasesCreateHandler called")

	if !isCanvasWriteEnabled() {
		return nil, errors.New(
			"canvas write tools are disabled by default. " +
				"To enable them, set the SLACK_MCP_CANVAS_WRITE_TOOL environment variable to 'true'")
	}

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	title := request.GetString("title", "")
	if title == "" {
		return nil, errors.New("title is required")
	}
	content := request.GetString("content", "")

	docContent := slack.DocumentContent{
		Type:     "markdown",
		Markdown: content,
	}

	canvasID, err := ch.apiProvider.Slack().CreateCanvasContext(ctx, title, docContent)
	if err != nil {
		ch.logger.Error("Failed to create canvas", zap.Error(err))
		return nil, fmt.Errorf("failed to create canvas: %w", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Canvas created successfully. ID: %s", canvasID)), nil
}

// CanvasesEditHandler edits an existing canvas (insert, replace, delete, rename).
func (ch *CanvasesHandler) CanvasesEditHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CanvasesEditHandler called")

	if !isCanvasWriteEnabled() {
		return nil, errors.New(
			"canvas write tools are disabled by default. " +
				"To enable them, set the SLACK_MCP_CANVAS_WRITE_TOOL environment variable to 'true'")
	}

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	canvasID := request.GetString("canvas_id", "")
	if canvasID == "" {
		return nil, errors.New("canvas_id is required")
	}

	operation := request.GetString("operation", "")
	if operation == "" {
		return nil, errors.New("operation is required (insert_after, insert_before, insert_at_start, insert_at_end, replace, delete, rename)")
	}

	sectionID := request.GetString("section_id", "")
	content := request.GetString("content", "")

	// Handle rename separately — it uses canvases.edit with a title-like approach
	// but actually the Slack API doesn't have a direct rename; we treat it as a full replace
	// of the document title by creating an edit with the appropriate operation
	if operation == "rename" {
		if content == "" {
			return nil, errors.New("content (new title) is required for rename operation")
		}
		// Slack doesn't have a direct rename API in canvases.edit.
		// We'll use canvases.edit with a replace operation on the entire content
		// as a workaround, but a better approach might be needed.
		// For now, treat rename as a special case.
		return nil, errors.New("rename operation is not directly supported by the Slack Canvas API. Use canvases_create with the desired title and copy content instead")
	}

	change := slack.CanvasChange{
		Operation: operation,
		DocumentContent: slack.DocumentContent{
			Type:     "markdown",
			Markdown: content,
		},
	}
	if sectionID != "" {
		change.SectionID = sectionID
	}

	params := slack.EditCanvasParams{
		CanvasID: canvasID,
		Changes:  []slack.CanvasChange{change},
	}

	err := ch.apiProvider.Slack().EditCanvasContext(ctx, params)
	if err != nil {
		ch.logger.Error("Failed to edit canvas",
			zap.String("canvas_id", canvasID),
			zap.String("operation", operation),
			zap.Error(err))
		return nil, fmt.Errorf("failed to edit canvas %s: %w", canvasID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Canvas %s edited successfully (operation: %s).", canvasID, operation)), nil
}
