// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/flagenum"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"
)

const (
	version   = "0.0.1"
	userAgent = "turboci cli v" + version
)

// Instructs the gRPC client how to retry.
var retryPolicy = fmt.Sprintf(`{
  "methodConfig": [{
    "name": [
      {"service": "%s"}
    ],
    "waitForReady": true,
    "retryPolicy": {
      "MaxAttempts": 5,
      "InitialBackoff": "0.01s",
      "MaxBackoff": "1s",
      "BackoffMultiplier": 2.0,
      "RetryableStatusCodes": [
        "INTERNAL",
        "UNAVAILABLE"
      ]
    }
  }]
}`,
	orchestratorgrpcpb.TurboCIOrchestrator_ServiceDesc.ServiceName,
)

// attachTokenMode is an enum with possible values for attachToken.
type attachTokenMode string

const (
	// Attach a token if not already in the request, but do not fail if there's
	// no token in the LUCI_CONTEXT.
	// Default value.
	attachTokenContext attachTokenMode = "context"
	// Attach a token if not already in the request, failing if there's no token
	// in the LUCI_CONTEXT.
	attachTokenAlways attachTokenMode = "always"
	// Do not do anything related to the token.
	attachTokenNever attachTokenMode = "never"
)

func (m *attachTokenMode) String() string {
	return string(*m)
}

func (m *attachTokenMode) Set(s string) error {
	return attachTokenChoices.FlagSet(m, s)
}

var attachTokenChoices = flagenum.Enum{
	"context": attachTokenContext,
	"always":  attachTokenAlways,
	"never":   attachTokenNever,
}

// baseCommandRun provides common command run functionality.
// All turboci subcommands must embed it directly or indirectly.
type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags   authcli.Flags
	apiEndpoint string

	attachToken attachTokenMode

	client         orchestratorgrpcpb.TurboCIOrchestratorClient
	turboCIContext *lucictx.TurboCI
}

// RegisterGlobalFlags registers the common flags.
func (r *baseCommandRun) RegisterGlobalFlags(p Params) {
	r.Flags.StringVar(
		&r.apiEndpoint, "endpoint", p.DefaultTurboCIHost, text.Doc(`
		Endpoint of the Turbo CI Orchestrator Private API.
	`))
	r.Flags.Var(
		&r.attachToken, "attach-token", text.Doc(`
		How to attach the TurboCI token from LUCI_CONTEXT to the request.

		Accepted values:
			* context: Attach a token if not already in the request, but do not fail if there's no token in the LUCI_CONTEXT. Default value.
			* always: Attach a token if not already in the request, failing if there's no token in the LUCI_CONTEXT.
			* never: Do not do anything related to the token.
		`),
	)
	r.authFlags.Register(&r.Flags, p.Auth)
}

// init creates the TurboCI client and sets turboCIContext.
func (r *baseCommandRun) init(ctx context.Context) error {
	if r.apiEndpoint == "" {
		return errors.New("missing --endpoint")
	}

	authOpts, err := r.authFlags.Options()
	if err != nil {
		return err
	}

	creds, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).PerRPCCredentials()
	if err != nil {
		return errors.Fmt("failed to get credentials: %w", err)
	}
	conn, err := grpc.NewClient(r.apiEndpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithUserAgent(userAgent),
		grpc.WithDefaultServiceConfig(retryPolicy),
	)
	if err != nil {
		return errors.Fmt("cannot dial to %s: %w", r.apiEndpoint, err)
	}
	r.client = orchestratorgrpcpb.NewTurboCIOrchestratorClient(conn)

	r.turboCIContext = lucictx.GetTurboCI(ctx)
	return nil
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

type request interface {
	GetToken() string
	SetToken(string)
}

func (r *baseCommandRun) attachTokenIfRequested(ctx context.Context, req request) error {
	if r.attachToken == attachTokenNever {
		return nil
	}

	if req.GetToken() != "" {
		logging.Infof(ctx,
			"Request already contains token, skipping attching the token from LUCI_CONTEXT")
		return nil
	}

	if r.turboCIContext == nil || r.turboCIContext.Token == "" {
		if r.attachToken == attachTokenAlways {
			return errors.New("missing TurboCI token in LUCI_CONTEXT")
		}

		logging.Infof(ctx,
			"No TurboCI token in LUCI_CONTEXT, skipping attching the token from LUCI_CONTEXT")
		return nil
	}

	req.SetToken(r.turboCIContext.Token)
	return nil
}
