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

package proxyserver

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"path/filepath"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

// ProxyParams is high-level configuration of the proxy.
type ProxyParams struct {
	// UnixSocket is a path to the Unix socket to serve the proxy on.
	UnixSocket string
	// Policy defines what actions are allowed to be performed through the proxy.
	Policy *proxypb.Policy
	// AuthenticatingClient will be used to contact remote CIPD backends.
	AuthenticatingClient *http.Client
	// UserAgent is the proxy's user agent, appended to the client's user agent.
	UserAgent string
	// Started is called once the proxy is listening to connections.
	Started func(proxyURL string)
	// AccessLog is called to record calls to the proxy RPC endpoints.
	AccessLog func(ctx context.Context, entry *cipdpb.AccessLogEntry)
	// CASOpLog is called to record number of bytes passed through the CAS proxy.
	CASOpLog func(ctx context.Context, op *CASOp)
}

// Run runs the CIPD proxy until SIGTERM.
//
// Returns an error if the server failed to start or died unexpectedly.
func Run(ctx context.Context, params ProxyParams) error {
	path, err := filepath.Abs(params.UnixSocket)
	if err != nil {
		return errors.Annotate(err, "bad socket path").Err()
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return errors.Annotate(err, "failed to initialize unix socket").Err()
	}

	obfuscator := NewCASURLObfuscator()

	srv := &Server{
		Listener: listener,
		Repository: &ProxyRepositoryServer{
			Policy:           params.Policy,
			RemoteFactory:    DefaultRemoteFactory(params.AuthenticatingClient),
			CASURLObfuscator: obfuscator,
			UserAgent:        params.UserAgent,
		},
		Storage: &cipdpb.UnimplementedStorageServer{},
		UnaryServerInterceptors: []grpc.UnaryServerInterceptor{
			AccessLogInterceptor(params.AccessLog),
			UnimplementedProxyInterceptor(),
		},
		CASURLObfuscator: obfuscator,
		CAS:              NewProxyCAS(params.Policy, params.CASOpLog),
	}

	stop := signals.HandleInterrupt(func() {
		logging.Infof(ctx, "Got a signal, shutting down the CIPD proxy...")
		if err := srv.Stop(ctx); err != nil {
			logging.Errorf(ctx, "Failed to shutdown: %s", err)
		}
	})
	defer stop()

	if params.Started != nil {
		params.Started((&url.URL{
			Scheme: "unix",
			Path:   filepath.ToSlash(path),
		}).String())
	}
	return srv.Serve(ctx)
}
