package provider

import (
	"sync/atomic"

	"github.com/korotovsky/slack-mcp-server/pkg/provider/lists"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// NewTestProvider creates an ApiProvider with a mock SlackAPI for unit testing.
// The provider is initialized as ready with empty caches.
func NewTestProvider(client SlackAPI, logger *zap.Logger) *ApiProvider {
	ap := &ApiProvider{
		transport:     "stdio",
		client:        client,
		listsClient:   nil,
		logger:        logger,
		rateLimiter:   rate.NewLimiter(rate.Inf, 0),
		usersReady:    true,
		channelsReady: true,
	}
	ap.usersSnapshot.Store(&UsersCache{
		Users:    make(map[string]slack.User),
		UsersInv: make(map[string]string),
	})
	ap.channelsSnapshot.Store(&ChannelsCache{
		Channels:    make(map[string]Channel),
		ChannelsInv: make(map[string]string),
	})
	return ap
}

// NewTestProviderWithLists creates an ApiProvider with both a mock SlackAPI and
// a ListsClient for testing list handlers.
func NewTestProviderWithLists(client SlackAPI, listsClient *lists.Client, logger *zap.Logger) *ApiProvider {
	ap := NewTestProvider(client, logger)
	ap.listsClient = listsClient
	return ap
}

// ensure atomic.Pointer is used (it's used via usersSnapshot/channelsSnapshot)
var _ atomic.Pointer[UsersCache]
