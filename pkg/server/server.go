package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/korotovsky/slack-mcp-server/pkg/handler"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/korotovsky/slack-mcp-server/pkg/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

type MCPServer struct {
	server *server.MCPServer
	logger *zap.Logger
}

func NewMCPServer(provider *provider.ApiProvider, logger *zap.Logger) *MCPServer {
	s := server.NewMCPServer(
		"Slack MCP Server",
		version.Version,
		server.WithLogging(),
		server.WithRecovery(),
		server.WithToolHandlerMiddleware(buildLoggerMiddleware(logger)),
		server.WithToolHandlerMiddleware(auth.BuildMiddleware(provider.ServerTransport(), logger)),
	)

	conversationsHandler := handler.NewConversationsHandler(provider, logger)

	s.AddTool(mcp.NewTool("conversations_history",
		mcp.WithDescription("Get messages from the channel (or DM) by channel_id, the last row/column in the response is used as 'cursor' parameter for pagination if not empty"),
		mcp.WithTitleAnnotation("Get Conversation History"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("    - `channel_id` (string): ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithBoolean("include_activity_messages",
			mcp.Description("If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false."),
			mcp.DefaultBool(false),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithString("limit",
			mcp.DefaultString("1d"),
			mcp.Description("Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided."),
		),
	), conversationsHandler.ConversationsHistoryHandler)

	s.AddTool(mcp.NewTool("conversations_replies",
		mcp.WithDescription("Get a thread of messages posted to a conversation by channelID and thread_ts, the last row/column in the response is used as 'cursor' parameter for pagination if not empty"),
		mcp.WithTitleAnnotation("Get Thread Replies"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Required(),
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread. ts must be the timestamp in format 1234567890.123456 of an existing message with 0 or more replies."),
		),
		mcp.WithBoolean("include_activity_messages",
			mcp.Description("If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false."),
			mcp.DefaultBool(false),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithString("limit",
			mcp.DefaultString("1d"),
			mcp.Description("Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided."),
		),
	), conversationsHandler.ConversationsRepliesHandler)

	s.AddTool(mcp.NewTool("conversations_add_message",
		mcp.WithDescription("Add a message to a public channel, private channel, or direct message (DM, or IM) conversation by channel_id and thread_ts."),
		mcp.WithTitleAnnotation("Send Message"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread_ts must be the timestamp in format 1234567890.123456 of an existing message with 0 or more replies. Optional, if not provided the message will be added to the channel itself, otherwise it will be added to the thread."),
		),
		mcp.WithString("payload",
			mcp.Description("Message payload in specified content_type format. Example: 'Hello, world!' for text/plain or '# Hello, world!' for text/markdown."),
		),
		mcp.WithString("content_type",
			mcp.DefaultString("text/markdown"),
			mcp.Description("Content type of the message. Default is 'text/markdown'. Allowed values: 'text/markdown', 'text/plain'."),
		),
	), conversationsHandler.ConversationsAddMessageHandler)

	s.AddTool(mcp.NewTool("reactions_add",
		mcp.WithDescription("Add an emoji reaction to a message in a public channel, private channel, or direct message (DM, or IM) conversation."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Timestamp of the message to add reaction to, in format 1234567890.123456."),
		),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("The name of the emoji to add as a reaction (without colons). Example: 'thumbsup', 'heart', 'rocket'."),
		),
	), conversationsHandler.ReactionsAddHandler)

	s.AddTool(mcp.NewTool("reactions_remove",
		mcp.WithDescription("Remove an emoji reaction from a message in a public channel, private channel, or direct message (DM, or IM) conversation."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Timestamp of the message to remove reaction from, in format 1234567890.123456."),
		),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("The name of the emoji to remove as a reaction (without colons). Example: 'thumbsup', 'heart', 'rocket'."),
		),
	), conversationsHandler.ReactionsRemoveHandler)

	s.AddTool(mcp.NewTool("attachment_get_data",
		mcp.WithDescription("Download an attachment's content by file ID. Returns file metadata and content (text files as-is, binary files as base64). Maximum file size is 5MB."),
		mcp.WithTitleAnnotation("Get Attachment Data"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_id",
			mcp.Required(),
			mcp.Description("The ID of the attachment to download, in format Fxxxxxxxxxx. Attachment IDs can be found in message metadata when HasMedia is true or AttachmentCount > 0."),
		),
	), conversationsHandler.FilesGetHandler)

	conversationsSearchTool := mcp.NewTool("conversations_search_messages",
		mcp.WithDescription("Search messages in a public channel, private channel, or direct message (DM, or IM) conversation using filters. All filters are optional, if not provided then search_query is required."),
		mcp.WithTitleAnnotation("Search Messages"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("search_query",
			mcp.Description("Search query to filter messages. Example: 'marketing report' or full URL of Slack message e.g. 'https://slack.com/archives/C1234567890/p1234567890123456', then the tool will return a single message matching given URL, herewith all other parameters will be ignored."),
		),
		mcp.WithString("filter_in_channel",
			mcp.Description("Filter messages in a specific public/private channel by its ID or name. Example: 'C1234567890', 'G1234567890', or '#general'. If not provided, all channels will be searched."),
		),
		mcp.WithString("filter_in_im_or_mpim",
			mcp.Description("Filter messages in a direct message (DM) or multi-person direct message (MPIM) conversation by its ID or name. Example: 'D1234567890' or '@username_dm'. If not provided, all DMs and MPIMs will be searched."),
		),
		mcp.WithString("filter_users_with",
			mcp.Description("Filter messages with a specific user by their ID or display name in threads and DMs. Example: 'U1234567890' or '@username'. If not provided, all threads and DMs will be searched."),
		),
		mcp.WithString("filter_users_from",
			mcp.Description("Filter messages from a specific user by their ID or display name. Example: 'U1234567890' or '@username'. If not provided, all users will be searched."),
		),
		mcp.WithString("filter_date_before",
			mcp.Description("Filter messages sent before a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_after",
			mcp.Description("Filter messages sent after a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_on",
			mcp.Description("Filter messages sent on a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_during",
			mcp.Description("Filter messages sent during a specific period in format 'YYYY-MM-DD'. Example: 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithBoolean("filter_threads_only",
			mcp.Description("If true, the response will include only messages from threads. Default is boolean false."),
		),
		mcp.WithString("cursor",
			mcp.DefaultString(""),
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(20),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 100."),
		),
	)
	// Only register search tool for non-bot tokens (bot tokens cannot use search.messages API)
	if !provider.IsBotToken() {
		s.AddTool(conversationsSearchTool, conversationsHandler.ConversationsSearchHandler)
	}

	s.AddTool(mcp.NewTool("users_search",
		mcp.WithDescription("Search for users by name, email, or display name. Returns user details and DM channel ID if available."),
		mcp.WithTitleAnnotation("Search Users"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query - matches against real name, display name, username, or email."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(10),
			mcp.Description("Maximum number of results to return (1-100). Default is 10."),
		),
	), conversationsHandler.UsersSearchHandler)

	canvasesHandler := handler.NewCanvasesHandler(provider, logger)

	s.AddTool(mcp.NewTool("canvases_list",
		mcp.WithDescription("List canvases in the workspace. Returns CSV with canvas IDs, titles, creators, and last updated timestamps."),
		mcp.WithTitleAnnotation("List Canvases"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("channel_id",
			mcp.Description("Optional channel ID to filter canvases by channel."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(100),
			mcp.Description("Maximum number of canvases to return (1-1000). Default is 100."),
		),
	), canvasesHandler.CanvasesListHandler)

	s.AddTool(mcp.NewTool("canvases_read",
		mcp.WithDescription("Read the content of a canvas by its ID. Returns the canvas content as markdown text."),
		mcp.WithTitleAnnotation("Read Canvas"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("canvas_id",
			mcp.Required(),
			mcp.Description("The ID of the canvas to read (starts with F)."),
		),
	), canvasesHandler.CanvasesReadHandler)

	s.AddTool(mcp.NewTool("canvases_sections_lookup",
		mcp.WithDescription("Find sections within a canvas by type or text content. Returns matching section IDs."),
		mcp.WithTitleAnnotation("Lookup Canvas Sections"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("canvas_id",
			mcp.Required(),
			mcp.Description("The ID of the canvas to search (starts with F)."),
		),
		mcp.WithString("contains_text",
			mcp.Description("Search for sections containing this text."),
		),
		mcp.WithString("section_types",
			mcp.Description("Comma-separated section types to filter by (e.g., 'any_header,sub_header')."),
		),
	), canvasesHandler.CanvasesSectionsLookupHandler)

	s.AddTool(mcp.NewTool("canvases_create",
		mcp.WithDescription("Create a new canvas with a title and markdown content. Requires SLACK_MCP_CANVAS_WRITE_TOOL=true."),
		mcp.WithTitleAnnotation("Create Canvas"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Title for the new canvas."),
		),
		mcp.WithString("content",
			mcp.Description("Markdown content for the canvas body."),
		),
	), canvasesHandler.CanvasesCreateHandler)

	s.AddTool(mcp.NewTool("canvases_edit",
		mcp.WithDescription("Edit an existing canvas. Supports operations: insert_after, insert_before, insert_at_start, insert_at_end, replace, delete. Requires SLACK_MCP_CANVAS_WRITE_TOOL=true."),
		mcp.WithTitleAnnotation("Edit Canvas"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("canvas_id",
			mcp.Required(),
			mcp.Description("The ID of the canvas to edit (starts with F)."),
		),
		mcp.WithString("operation",
			mcp.Required(),
			mcp.Description("Edit operation: insert_after, insert_before, insert_at_start, insert_at_end, replace, delete."),
		),
		mcp.WithString("section_id",
			mcp.Description("Section ID to target. Required for insert_after, insert_before, delete. Optional for replace (omit to replace entire body)."),
		),
		mcp.WithString("content",
			mcp.Description("Markdown content for insert/replace operations."),
		),
	), canvasesHandler.CanvasesEditHandler)

	listsHandler := handler.NewListsHandler(provider, logger)

	s.AddTool(mcp.NewTool("lists_get_items",
		mcp.WithDescription("Get items from a Slack list. Returns CSV with column headers matching the list schema. Supports cursor-based pagination."),
		mcp.WithTitleAnnotation("Get List Items"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("list_id",
			mcp.Required(),
			mcp.Description("The ID of the list (starts with F)."),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination from a previous response."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(100),
			mcp.Description("Maximum number of items to return. Default is 100."),
		),
	), listsHandler.ListsGetItemsHandler)

	s.AddTool(mcp.NewTool("lists_get_item",
		mcp.WithDescription("Get a single item from a Slack list by record ID. Returns the item's fields in a readable format."),
		mcp.WithTitleAnnotation("Get List Item"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("list_id",
			mcp.Required(),
			mcp.Description("The ID of the list (starts with F)."),
		),
		mcp.WithString("record_id",
			mcp.Required(),
			mcp.Description("The ID of the record to retrieve (starts with Rec)."),
		),
	), listsHandler.ListsGetItemHandler)

	s.AddTool(mcp.NewTool("lists_add_item",
		mcp.WithDescription("Add a new item to a Slack list. Fields are provided as a JSON object mapping column IDs to values. Text values are automatically wrapped in the required Block Kit format. Requires SLACK_MCP_LIST_WRITE_TOOL=true."),
		mcp.WithTitleAnnotation("Add List Item"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("list_id",
			mcp.Required(),
			mcp.Description("The ID of the list (starts with F)."),
		),
		mcp.WithString("fields",
			mcp.Required(),
			mcp.Description("JSON object mapping column IDs to values. Example: {\"Col001\": \"Task title\", \"Col002\": \"high\"}"),
		),
	), listsHandler.ListsAddItemHandler)

	s.AddTool(mcp.NewTool("lists_update_item",
		mcp.WithDescription("Update a specific field in a Slack list item. Text values are automatically wrapped in Block Kit format. Requires SLACK_MCP_LIST_WRITE_TOOL=true."),
		mcp.WithTitleAnnotation("Update List Item"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("list_id",
			mcp.Required(),
			mcp.Description("The ID of the list (starts with F)."),
		),
		mcp.WithString("record_id",
			mcp.Required(),
			mcp.Description("The ID of the record to update (starts with Rec)."),
		),
		mcp.WithString("column_id",
			mcp.Required(),
			mcp.Description("The ID of the column to update (starts with Col)."),
		),
		mcp.WithString("value",
			mcp.Description("The new value for the field."),
		),
	), listsHandler.ListsUpdateItemHandler)

	s.AddTool(mcp.NewTool("lists_delete_item",
		mcp.WithDescription("Delete an item from a Slack list. Requires SLACK_MCP_LIST_WRITE_TOOL=true."),
		mcp.WithTitleAnnotation("Delete List Item"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("list_id",
			mcp.Required(),
			mcp.Description("The ID of the list (starts with F)."),
		),
		mcp.WithString("record_id",
			mcp.Required(),
			mcp.Description("The ID of the record to delete (starts with Rec)."),
		),
	), listsHandler.ListsDeleteItemHandler)

	channelsHandler := handler.NewChannelsHandler(provider, logger)

	s.AddTool(mcp.NewTool("channels_list",
		mcp.WithDescription("Get list of channels"),
		mcp.WithTitleAnnotation("List Channels"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("channel_types",
			mcp.Required(),
			mcp.Description("Comma-separated channel types. Allowed values: 'mpim', 'im', 'public_channel', 'private_channel'. Example: 'public_channel,private_channel,im'"),
		),
		mcp.WithString("sort",
			mcp.Description("Type of sorting. Allowed values: 'popularity' - sort by number of members/participants in each channel."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(100),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 1000 (maximum 999)."), // context fix for cursor: https://github.com/korotovsky/slack-mcp-server/issues/7
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
	), channelsHandler.ChannelsHandler)

	logger.Info("Authenticating with Slack API...",
		zap.String("context", "console"),
	)
	ar, err := provider.Slack().AuthTest()
	if err != nil {
		logger.Fatal("Failed to authenticate with Slack",
			zap.String("context", "console"),
			zap.Error(err),
		)
	}

	logger.Info("Successfully authenticated with Slack",
		zap.String("context", "console"),
		zap.String("team", ar.Team),
		zap.String("user", ar.User),
		zap.String("enterprise", ar.EnterpriseID),
		zap.String("url", ar.URL),
	)

	ws, err := text.Workspace(ar.URL)
	if err != nil {
		logger.Fatal("Failed to parse workspace from URL",
			zap.String("context", "console"),
			zap.String("url", ar.URL),
			zap.Error(err),
		)
	}

	s.AddResource(mcp.NewResource(
		"slack://"+ws+"/channels",
		"Directory of Slack channels",
		mcp.WithResourceDescription("This resource provides a directory of Slack channels."),
		mcp.WithMIMEType("text/csv"),
	), channelsHandler.ChannelsResource)

	s.AddResource(mcp.NewResource(
		"slack://"+ws+"/users",
		"Directory of Slack users",
		mcp.WithResourceDescription("This resource provides a directory of Slack users."),
		mcp.WithMIMEType("text/csv"),
	), conversationsHandler.UsersResource)

	return &MCPServer{
		server: s,
		logger: logger,
	}
}

func (s *MCPServer) ServeSSE(addr string) *server.SSEServer {
	s.logger.Info("Creating SSE server",
		zap.String("context", "console"),
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
		zap.String("address", addr),
	)
	return server.NewSSEServer(s.server,
		server.WithBaseURL(fmt.Sprintf("http://%s", addr)),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			ctx = auth.AuthFromRequest(s.logger)(ctx, r)

			return ctx
		}),
	)
}

func (s *MCPServer) ServeHTTP(addr string) *server.StreamableHTTPServer {
	s.logger.Info("Creating HTTP server",
		zap.String("context", "console"),
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
		zap.String("address", addr),
	)
	return server.NewStreamableHTTPServer(s.server,
		server.WithEndpointPath("/mcp"),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			ctx = auth.AuthFromRequest(s.logger)(ctx, r)

			return ctx
		}),
	)
}

func (s *MCPServer) ServeStdio() error {
	s.logger.Info("Starting STDIO server",
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
	)
	err := server.ServeStdio(s.server)
	if err != nil {
		s.logger.Error("STDIO server error", zap.Error(err))
	}
	return err
}

func buildLoggerMiddleware(logger *zap.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			logger.Info("Request received",
				zap.String("tool", req.Params.Name),
				zap.Any("params", req.Params),
			)

			startTime := time.Now()

			res, err := next(ctx, req)

			duration := time.Since(startTime)

			logger.Info("Request finished",
				zap.String("tool", req.Params.Name),
				zap.Duration("duration", duration),
			)

			return res, err
		}
	}
}
