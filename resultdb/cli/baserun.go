// Copyright 2019 The LUCI Authors.
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
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ExitCodeCommandFailure indicates that a given command failed due to internal errors
// or invalid input parameters.
const ExitCodeCommandFailure = 123

// baseCommandRun provides common command run functionality.
// All rdb subcommands must embed it directly or indirectly.
type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags         authcli.Flags
	host              string
	json              bool
	forceInsecure     bool
	fallbackHost      string
	loggingConfig     logging.Config
	maxConcurrentRPCs int

	http        *http.Client
	prpcClient  *prpc.Client
	resultdb    pb.ResultDBClient
	schemas     pb.SchemasClient
	recorder    pb.RecorderClient
	resultdbCtx *lucictx.ResultDB
}

func (r *baseCommandRun) RegisterGlobalFlags(p Params) {
	r.Flags.StringVar(&r.host, "host", "", text.Doc(`
		Host of the resultdb instance. Overrides the one in LUCI_CONTEXT.
	`))
	r.Flags.BoolVar(&r.forceInsecure, "force-insecure", false, text.Doc(`
		Force HTTP, as opposed to HTTPS.
	`))
	r.Flags.IntVar(&r.maxConcurrentRPCs, "max-concurrent-rpcs", 20, text.Doc(`
		Max concurrent RPCs to the resultdb instance (0 for unlimited).
	`))
	r.authFlags.Register(&r.Flags, p.Auth)
	// Copy the given default to the struct s.t. initClients
	// can use it if needed.
	r.fallbackHost = p.DefaultResultDBHost
	r.loggingConfig.Level = logging.Info
	r.loggingConfig.AddFlags(&r.Flags)
}

func (r *baseCommandRun) RegisterJSONFlag(usage string) {
	r.Flags.BoolVar(&r.json, "json", false, usage)
}

func (r *baseCommandRun) ModifyContext(ctx context.Context) context.Context {
	return r.loggingConfig.Set(ctx)
}

// initClients validates -host flag and initializes r.httpClient, r.resultdb and
// r.recorder.
func (r *baseCommandRun) initClients(ctx context.Context, loginMode auth.LoginMode) error {
	// Create HTTP Client.
	authOpts, err := r.authFlags.Options()
	if err != nil {
		return err
	}
	r.http, err = auth.NewAuthenticator(ctx, loginMode, authOpts).Client()
	if err != nil {
		return err
	}

	r.resultdbCtx = lucictx.GetResultDB(ctx)

	// If no host specified in command line populate from lucictx.
	// If host also not set in lucictx, fall back to r.fallbackHost.
	if r.host == "" {
		if r.resultdbCtx != nil && r.resultdbCtx.Hostname != "" {
			r.host = r.resultdbCtx.Hostname
		} else {
			r.host = r.fallbackHost
		}
	}

	// Validate -host
	if r.host == "" {
		return fmt.Errorf("a host for resultdb is required")
	}
	if strings.ContainsRune(r.host, '/') {
		return fmt.Errorf("invalid host %q", r.host)
	}

	if r.maxConcurrentRPCs < 0 {
		return fmt.Errorf("invalid -max-concurrent-rpcs %d", r.maxConcurrentRPCs)
	}

	// Create clients.
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = r.forceInsecure || lhttp.IsLocalHost(r.host)
	info, err := version.GetCurrentVersion()
	if err != nil {
		return err
	}
	rpcOpts.UserAgent = fmt.Sprintf("resultdb CLI, instanceID=%q", info.InstanceID)
	r.prpcClient = &prpc.Client{
		C:                     r.http,
		Host:                  r.host,
		Options:               rpcOpts,
		MaxConcurrentRequests: r.maxConcurrentRPCs,
	}
	r.resultdb = pb.NewResultDBPRPCClient(r.prpcClient)
	r.schemas = pb.NewSchemasClient(r.prpcClient)
	r.recorder = pb.NewRecorderPRPCClient(r.prpcClient)
	return nil
}

func (r *baseCommandRun) validateCurrentInvocation() error {
	if r.resultdbCtx == nil {
		return errors.New("resultdb section of LUCI_CONTEXT missing")
	}

	if r.resultdbCtx.CurrentInvocation.Name == "" {
		return errors.New("current invocation name missing from LUCI_CONTEXT")
	}

	if r.resultdbCtx.CurrentInvocation.UpdateToken == "" {
		return errors.New("invocation update token missing from LUCI_CONTEXT")
	}
	return nil
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeCommandFailure
	}
	return 0
}
