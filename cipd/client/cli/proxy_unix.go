// Copyright 2025 The LUCI Authors.
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

//go:build !windows
// +build !windows

package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/dustin/go-humanize"

	"go.chromium.org/luci/common/logging"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

func runProxyImpl(ctx context.Context, unixSocket string, policy *proxypb.Policy, authClient *http.Client) (*proxyserver.ProxyStats, error) {
	stats := &proxyserver.ProxyStats{}
	defer stats.Report(ctx)

	ua := strings.Replace(cipd.UserAgent, "cipd ", "cipd-proxy ", 1)
	return stats, proxyserver.Run(ctx, proxyserver.ProxyParams{
		UnixSocket:           unixSocket,
		Policy:               policy,
		AuthenticatingClient: authClient,
		UserAgent:            ua,
		Started: func(proxyURL string) {
			logging.Infof(ctx, "Launching %s", ua)
			_, _ = fmt.Fprintf(os.Stdout, "%s=%s\n", cipd.EnvCIPDProxyURL, proxyURL)
			_ = os.Stdout.Close()
		},
		AccessLog: func(ctx context.Context, entry *cipdpb.AccessLogEntry) {
			logging.Infof(ctx, "RPC %s", entry.Method)
			stats.RPCCall(entry)
		},
		CASOpLog: func(ctx context.Context, op *proxyserver.CASOp) {
			switch {
			case op.Downloaded != 0:
				logging.Infof(ctx, "Downloaded %s", humanize.Bytes(op.Downloaded))
			case op.Uploaded != 0:
				logging.Infof(ctx, "Uploaded %s", humanize.Bytes(op.Uploaded))
			}
			stats.CASOp(op)
		},
	})
}
