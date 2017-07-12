// Copyright 2017 The LUCI Authors.
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

// +build include_profiler

package cli

import (
	"context"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/runtime/profiling"
)

type profilingExt struct {
	added bool
}

func (p *profilingExt) addProfiling(cmds []*subcommands.Command) {
	if !p.added {
		for _, cmd := range cmds {
			cmd.CommandRun = wrapCmdRun(cmd.CommandRun)
		}
		p.added = true
	}
}

func wrapCmdRun(orig func() subcommands.CommandRun) func() subcommands.CommandRun {
	return func() subcommands.CommandRun {
		r := &wrappedCmdRun{CommandRun: orig()}
		r.prof.AddFlags(r.GetFlags())
		return r
	}
}

type wrappedCmdRun struct {
	subcommands.CommandRun
	prof profiling.Profiler
}

// Run is part of CommandRun interface.
func (r *wrappedCmdRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := GetContext(a, r, env)
	r.prof.Logger = logging.Get(ctx)
	r.prof.Clock = clock.Get(ctx)

	if err := r.prof.Start(); err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to start profiling")
		return 1
	}
	defer r.prof.Stop()

	return r.CommandRun.Run(a, args, env)
}

// ModifyContext is part of ContextModificator interface.
//
// Need to explicitly define it, since embedding original CommandRun in
// wrappedCmdRun "disables" the sniffing of ContextModificator in GetContext.
func (r *wrappedCmdRun) ModifyContext(ctx context.Context) context.Context {
	if m, _ := r.CommandRun.(ContextModificator); m != nil {
		return m.ModifyContext(ctx)
	}
	return ctx
}
