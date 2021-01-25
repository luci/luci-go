package lib

import (
	"context"
	"net/http"

	"go.chromium.org/luci/auth"
)

// createAuthClient returns a new autheticator.
func createAuthClient(ctx context.Context, authOpts *auth.Options) (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP-allowed bots: they have NO credentials to send.
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, *authOpts).Client()
}
