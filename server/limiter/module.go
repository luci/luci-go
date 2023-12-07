// Copyright 2020 The LUCI Authors.
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

package limiter

import (
	"context"
	"flag"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/limiter")

const defaultMaxConcurrentRPCs = 100000 // e.g. unlimited

// ModuleOptions contains configuration of the server module that installs
// default limiters applied to all routes/services in the server.
type ModuleOptions struct {
	MaxConcurrentRPCs int64 // limit on a number of incoming concurrent RPCs (default is 100000, i.e. unlimited)
	AdvisoryMode      bool  // if set, don't enforce MaxConcurrentRPCs, but still report violations
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	if o.MaxConcurrentRPCs == 0 {
		o.MaxConcurrentRPCs = defaultMaxConcurrentRPCs
	}
	f.Int64Var(
		&o.MaxConcurrentRPCs,
		"limiter-max-concurrent-rpcs",
		o.MaxConcurrentRPCs,
		fmt.Sprintf("Limit on a number of incoming concurrent RPCs (default is %d)", o.MaxConcurrentRPCs),
	)
	f.BoolVar(
		&o.AdvisoryMode,
		"limiter-advisory-mode",
		o.AdvisoryMode,
		"If set, don't enforce -limiter-max-concurrent-rpcs, but still report violations",
	)
}

// NewModule returns a server module that installs default limiters applied to
// all routes/services in the server.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &serverModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// serverModule implements module.Module.
type serverModule struct {
	opts       *ModuleOptions
	rpcLimiter *Limiter // limits in-flight RPCs in the main port
}

// Name is part of module.Module interface.
func (*serverModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*serverModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	// Now that options are parsed by the server we can validate them and
	// construct a limiter from them.
	if m.opts.MaxConcurrentRPCs == 0 {
		m.opts.MaxConcurrentRPCs = defaultMaxConcurrentRPCs
	}
	var err error
	m.rpcLimiter, err = New(Options{
		Name:                  "rpc",
		AdvisoryMode:          m.opts.AdvisoryMode,
		MaxConcurrentRequests: m.opts.MaxConcurrentRPCs,
	})
	if err != nil {
		return nil, err
	}

	// We want limiter's metrics to be reported before every flush (so the flushed
	// values are as fresh as possible) and also once per second (to make the
	// limiter state observable through /admin/portal/tsmon/metrics debug UI).
	tsmon.RegisterCallbackIn(ctx, m.rpcLimiter.ReportMetrics)
	host.RunInBackground("luci.limiter.rpc", func(ctx context.Context) {
		for {
			m.rpcLimiter.ReportMetrics(ctx)
			if r := <-clock.After(ctx, time.Second); r.Err != nil {
				return // the context is canceled
			}
		}
	})

	// Actually add the limiter to the default interceptors chains.
	intr := NewServerInterceptor(m.rpcLimiter)
	host.RegisterUnaryServerInterceptors(intr.Unary())
	host.RegisterStreamServerInterceptors(intr.Stream())
	return ctx, nil
}
