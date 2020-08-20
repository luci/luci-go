package cas

import (
	"context"
	"strings"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func NewClient(ctx context.Context, instance string, tokenServerHost string, readOnly bool) (*client.Client, error) {
	project := strings.Split(instance, "/")[1]
	var role string
	if readOnly {
		role = "cas-read-only"
	} else {
		role = "cas-read-write"
	}

	// Construct auth.Options.
	opts := chromeinfra.DefaultAuthOptions()
	opts.TokenServerHost = tokenServerHost
	opts.ActAsServiceAccount = role + "@" + project + ".iam.gserviceaccount.com"
	opts.ActViaLUCIRealm = "@internal:" + project + "/" + role
	opts.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}

	a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	creds, err := a.PerRPCCredentials()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
	}

	client, err := client.NewClient(ctx, instance,
		client.DialParams{
			Service:            "remotebuildexecution.googleapis.com:443",
			TransportCredsOnly: true,
		}, &client.PerRPCCreds{Creds: creds})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create client").Err()
	}

	return client, nil
}
