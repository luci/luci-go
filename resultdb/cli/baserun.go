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
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/grpc/prpc"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// baseCommandRun provides common command run functionality.
// All rdb subcommands must embed it directly or indirectly.
type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags     authcli.Flags
	host          string
	json          bool
	forceInsecure bool

	http     *http.Client
	resultdb pb.ResultDBClient
	recorder pb.RecorderClient
}

func (r *baseCommandRun) RegisterGlobalFlags(p Params) {
	r.Flags.StringVar(&r.host, "host", p.DefaultResultDBHost, text.Doc(`
		Host of the resultdb instance.
	`))
	r.Flags.BoolVar(&r.forceInsecure, "force-insecure", false, text.Doc(`
		Force HTTP, as opposed to HTTPS.
	`))
	r.authFlags.Register(&r.Flags, p.Auth)
}

func (r *baseCommandRun) RegisterJSONFlag(usage string) {
	r.Flags.BoolVar(&r.json, "json", false, usage)
}

// initClients validates -host flag and initializes r.httpClient, r.resultdb
// and r.recorder.
func (r *baseCommandRun) initClients(ctx context.Context) error {
	// Create HTTP Client.
	authOpts, err := r.authFlags.Options()
	if err != nil {
		return err
	}
	r.http, err = auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return err
	}

	// Validate -host
	if r.host == "" {
		return fmt.Errorf("a host for resuldb is required")
	}
	if strings.ContainsRune(r.host, '/') {
		return fmt.Errorf("invalid host %q", r.host)
	}

	// Create clients.
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = r.forceInsecure || lhttp.IsLocalHost(r.host)
	info, err := version.GetCurrentVersion()
	if err != nil {
		return err
	}
	rpcOpts.UserAgent = fmt.Sprintf("resultdb CLI, instanceID=%q", info.InstanceID)
	prpcClient := &prpc.Client{
		C:       r.http,
		Host:    r.host,
		Options: rpcOpts,
	}
	r.resultdb = pb.NewResultDBPRPCClient(prpcClient)
	r.recorder = pb.NewRecorderPRPCClient(prpcClient)
	return nil
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
