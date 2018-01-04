package git

import (
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// ClientFactory creates a Gitiles client.
type ClientFactory func(ctx context.Context, host string) (gitilespb.GitilesClient, error)

var factoryKey = "gitiles client factory key"

// UseFactory installs f into c.
func UseFactory(c context.Context, f ClientFactory) context.Context {
	return context.WithValue(c, &factoryKey, f)
}

// AuthenticatedProdClient returns a production Gitiles client,
// authenticated as self. Implements ClientFactory.
func AuthenticatedProdClient(c context.Context, host string) (gitilespb.GitilesClient, error) {
	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}

	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// Client creates a new Gitiles client using the ClientFactory installed in c.
// See also UseFactory.
func Client(c context.Context, host string) (gitilespb.GitilesClient, error) {
	f, ok := c.Value(&factoryKey).(ClientFactory)
	if !ok {
		return nil, errors.New("gitiles client factory is not installed in context")
	}
	return f(c, host)
}
