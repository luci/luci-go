// Copyright 2023 The LUCI Authors.
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

package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.chromium.org/luci/grpc/grpcutil"
)

// grpcPort implements servingPort on top of a gRPC server.
type grpcPort struct {
	listener net.Listener // the bound socket, always non-nil, doesn't change

	m         sync.Mutex          // protects mutable state below
	services  []grpcService       // services to register when starting
	opts      []grpc.ServerOption // options to pass to the server when starting
	server    *grpc.Server        // non-nil after the server has started
	healthSrv *health.Server      // the basic health check service
}

// grpcService is a service registered via registerService.
type grpcService struct {
	desc *grpc.ServiceDesc
	impl any
}

// registerService exposes the given service over gRPC.
func (p *grpcPort) registerService(desc *grpc.ServiceDesc, impl any) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.server != nil {
		panic("the server has already started")
	}
	p.services = append(p.services, grpcService{desc, impl})
}

// addServerOptions adds an option to pass to NewServer.
func (p *grpcPort) addServerOptions(opts ...grpc.ServerOption) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.server != nil {
		panic("the server has already started")
	}
	p.opts = append(p.opts, opts...)
}

// nameForLog returns a string to identify this port in the server logs.
//
// Part of the servingPort interface.
func (p *grpcPort) nameForLog() string {
	return fmt.Sprintf("grpc://%s [grpc]", p.listener.Addr())
}

// serve runs the serving loop until it is gracefully stopped.
//
// Part of the servingPort interface.
func (p *grpcPort) serve(baseCtx func() context.Context) error {
	p.m.Lock()
	if p.server != nil {
		panic("the server has already started")
	}

	// Install outer-most interceptors that append the root server context values
	// to the per-request context (which is basically just a context.Background()
	// with its cancellation controlled by the gRPC server). NewServer will append
	// other interceptors in `opts` after these root ones.
	injectCtx := contextInjector(baseCtx)
	p.opts = append(p.opts,
		grpc.UnaryInterceptor(injectCtx.Unary()),
		grpc.StreamInterceptor(injectCtx.Stream()),
	)
	p.server = grpc.NewServer(p.opts...)

	// Install reflection only into gRPC server (not pRPC one), since it uses
	// streaming RPCs not supported by pRPC. pRPC has its own similar service
	// called Discovery.
	reflection.Register(p.server)

	// Services installed into both pRPC and gRPC.
	hasHealth := false
	for _, svc := range p.services {
		p.server.RegisterService(svc.desc, svc.impl)
		if strings.HasPrefix(svc.desc.ServiceName, "grpc.health.") {
			hasHealth = true
		}
	}

	// If the health check service not installed yet by the server user, install
	// our own basic one. The server users might want to install their own
	// implementations if they plan to dynamically control the serving status.
	if !hasHealth {
		p.healthSrv = health.NewServer()
		grpc_health_v1.RegisterHealthServer(p.server, p.healthSrv)
	}

	server := p.server
	p.m.Unlock()

	return server.Serve(p.listener)
}

// shutdown gracefully stops the server, blocking until it is closed.
//
// Does nothing if the server is not running.
//
// Part of the servingPort interface.
func (p *grpcPort) shutdown(ctx context.Context) {
	p.m.Lock()
	server := p.server
	if p.healthSrv != nil {
		p.healthSrv.Shutdown() // announce we are going away
	}
	p.m.Unlock()
	if server != nil {
		server.GracefulStop()
	}
}

////////////////////////////////////////////////////////////////////////////////

// contextInjector is an interceptor that replaces a context with the one that
// takes values from the request context **and** baseCtx(), but keeps
// cancellation of the request context.
func contextInjector(baseCtx func() context.Context) grpcutil.UnifiedServerInterceptor {
	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
		return handler(&mergedCtx{ctx, baseCtx()})
	}
}

type mergedCtx struct {
	context.Context
	values context.Context
}

func (m mergedCtx) Value(key any) any {
	if v := m.Context.Value(key); v != nil {
		return v
	}
	return m.values.Value(key)
}
