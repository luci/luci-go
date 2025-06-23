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

// Package proxyserver contains CIPD proxy server base protocol.
package proxyserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

// ProxiedCASDomain is the expected domain of all requests to the CAS storage
// sent by the proxy client.
//
// Requests from the proxy client sent to "cipd.local" will be treated as
// requests to fetch an object from the CAS. It is responsibility of the
// cipdpb.RepositoryServer and/or cipdpb.StorageServer implementations to
// make sure all object URLs point to this domain by using an instance of
// CASURLObfuscator to generate them.
const ProxiedCASDomain = "cipd.local"

// CASHandler handles all requests (POST and GET) to obfuscated CAS object URLs.
type CASHandler func(obj *proxypb.ProxiedCASObject, rw http.ResponseWriter, req *http.Request)

// Server implements the server side of the proxy protocol.
//
// All pRPC requests from proxy clients will be forwarded to the given service
// implementations (which supposedly then call real remote RPCs). These
// implementations should use the CASURLObfuscator to generate proxy-local
// CAS object URLs. When the proxy client hits them, they would end up in calls
// to `CAS` handler that can actually serve raw bytes.
type Server struct {
	// Listener to accept incoming connections.
	Listener net.Listener

	// Repository is the implementation of the cipd.Repository RPC service.
	Repository cipdpb.RepositoryServer
	// Storage is the implementation of the cipd.Storage RPC service.
	Storage cipdpb.StorageServer

	// UnaryServerInterceptors are applied to all incoming RPCs.
	UnaryServerInterceptors []grpc.UnaryServerInterceptor

	// CASObfuscator does CAS object URL unobfuscation.
	CASURLObfuscator *CASURLObfuscator
	// CAS handles all requests (POST and GET) to obfuscated CAS object URLs.
	CAS CASHandler

	m       sync.Mutex
	httpSrv *http.Server  // the actual serving server
	prpcSrv *prpc.Server  // serves pRPC protocol
	stopped chan struct{} // closed after graceful shutdown
	done    bool          // true if `stopped` was already closed
}

// Serve blocks running the serving loop.
//
// Uses the given context as the base context for all requests.
//
// Returns nil some time after Stop is called once all in-flight requests are
// done. Returns an error if the server failed to start or died unexpectedly.
func (srv *Server) Serve(ctx context.Context) error {
	httpSrv, stopped, err := srv.initialize(ctx)
	if err != nil {
		return err
	}
	// This blocks as long as the listening port is listening.
	if err = httpSrv.Serve(srv.Listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// Wait until all in-flight requests are done.
	<-stopped
	return nil
}

// Stop initiates the server shutdown and waits for it to finish.
//
// At some point it will unblock Serve.
func (srv *Server) Stop(ctx context.Context) error {
	srv.m.Lock()
	done := srv.done
	httpSrv := srv.httpSrv
	srv.m.Unlock()
	if done || httpSrv == nil {
		return nil
	}
	// This blocks until all in-flight requests are done.
	if err := httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	// Notify Serve it can return now.
	srv.m.Lock()
	if !srv.done {
		srv.done = true
		close(srv.stopped)
	}
	srv.m.Unlock()
	return nil
}

// initialize initializes server guts.
func (srv *Server) initialize(ctx context.Context) (*http.Server, chan struct{}, error) {
	srv.m.Lock()
	defer srv.m.Unlock()
	if srv.httpSrv != nil {
		return nil, nil, errors.New("already started")
	}

	srv.prpcSrv = &prpc.Server{
		UnaryServerInterceptor: grpcutil.ChainUnaryServerInterceptors(srv.UnaryServerInterceptors...),
		ResponseCompression:    prpc.CompressNever,
	}
	cipdpb.RegisterRepositoryServer(srv.prpcSrv, srv.Repository)
	cipdpb.RegisterStorageServer(srv.prpcSrv, srv.Storage)

	prpcRouter := router.New()
	srv.prpcSrv.InstallHandlers(prpcRouter, nil)

	srv.httpSrv = &http.Server{
		Addr:        srv.Listener.Addr().String(), // just for logs
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler: h2c.NewHandler(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
				// ErrAbortHandler is explicitly used by net/http to signal that the panic
				// is "expected" and it should not be logged.
				if p.Reason != http.ErrAbortHandler {
					p.Log(ctx, "Caught panic during handling of %q: %s", req.RequestURI, p.Reason)
					http.Error(rw, "Internal Server Error. See logs.", http.StatusInternalServerError)
				}
			})
			if req.Host == ProxiedCASDomain {
				url := fmt.Sprintf("http://%s%s", req.Host, req.URL)
				obj, err := srv.CASURLObfuscator.Unobfuscate(url)
				if err != nil {
					logging.Errorf(ctx, "Failed to unobfuscate URL %q: %s", url, err)
					http.Error(rw, fmt.Sprintf("Invalid CAS object URL: %s", err), http.StatusForbidden)
				} else {
					srv.CAS(obj, rw, req)
				}
			} else {
				prpcRouter.ServeHTTP(rw, req)
			}
		}), &http2.Server{}),
	}

	srv.stopped = make(chan struct{})
	return srv.httpSrv, srv.stopped, nil
}

// TargetHost return hostname of the remote CIPD service the proxy client is
// hitting through the proxy or "" if unknown.
func TargetHost(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	if v := md.Get(":authority"); len(v) != 0 {
		return v[0]
	}
	if v := md.Get("host"); len(v) != 0 {
		return v[0]
	}
	return ""
}
