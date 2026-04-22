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
	"io"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'proxy' subcommand.

func cmdProxy(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "proxy <flags>",
		ShortDesc: "Runs a local CIPD proxy",
		LongDesc:  "Runs a local CIPD proxy.",
		CommandRun: func() subcommands.CommandRun {
			c := &proxyRun{}
			c.registerBaseFlags()
			c.authFlags.Register(&c.Flags, params.DefaultAuthOptions)
			c.authFlags.RegisterCredentialHelperFlags(&c.Flags)
			c.authFlags.RegisterADCFlags(&c.Flags)
			c.Flags.StringVar(&c.unixSocket, "unix-socket", "", "Unix domain socket path to serve the proxy on (default to a temp file).")
			c.Flags.StringVar(&c.proxyPolicy, "proxy-policy", "-", "Path to a text protobuf file with cipd.proxy.Policy message (or - for reading from stdin).")
			return c
		},
	}
}

type proxyRun struct {
	cipdSubcommand
	authFlags authcli.Flags

	unixSocket  string // -unix-socket flag
	proxyPolicy string // -proxy-policy flag
}

func (c *proxyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(runProxy(ctx, c.authFlags, c.unixSocket, c.proxyPolicy))
}

func runProxy(ctx context.Context, authFlags authcli.Flags, unixSocket, proxyPolicy string) (*proxyserver.ProxyStats, error) {
	if unixSocket == "" {
		dir, err := os.MkdirTemp("", "cipd")
		if err != nil {
			return nil, cipderr.IO.Apply(errors.Fmt("failed to create a temp dir: %w", err))
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				logging.Warningf(ctx, "Failed to cleanup CIPD proxy temp dir: %s", err)
			}
		}()
		unixSocket = filepath.Join(dir, "proxy.unix")
	}
	policy, err := readProxyPolicy(ctx, proxyPolicy)
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("bad proxy policy file: %w", err))
	}
	authOpts, err := authFlags.Options()
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("bad auth options: %w", err))
	}
	authClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, cipderr.Auth.Apply(errors.Fmt("initializing auth client: %w", err))
	}
	return runProxyImpl(ctx, unixSocket, policy, authClient)
}

func readProxyPolicy(ctx context.Context, path string) (*proxypb.Policy, error) {
	var file *os.File
	if path == "-" {
		logging.Infof(ctx, "Reading CIPD proxy policy from stdin...")
		file = os.Stdin
	} else {
		var err error
		file, err = os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, cipderr.BadArgument.Apply(errors.Fmt("missing proxy policy file: %w", err))
			}
			return nil, cipderr.IO.Apply(errors.Fmt("reading proxy policy file: %w", err))
		}
		defer func() { _ = file.Close() }()
	}

	blob, err := io.ReadAll(file)
	if err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("reading proxy policy file: %w", err))
	}

	var policy proxypb.Policy
	if err := (prototext.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(blob, &policy); err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("malformed proxy policy file: %w", err))
	}
	return &policy, nil
}
