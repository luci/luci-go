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

package cfgcache

import (
	"context"
	"flag"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
)

func TestServerModule(t *testing.T) {
	t.Parallel()

	newTestOpts := func() *ModuleOptions {
		v := &vars.VarSet{}
		v.Register("appid", func(context.Context) (string, error) { return "my-app", nil })
		rules := &validation.RuleSet{Vars: v}
		disp := &cron.Dispatcher{}
		return &ModuleOptions{Registry: &Registry{}, Vars: v, Rules: rules, CronDispatcher: disp}
	}

	t.Run("default options install validation rules, cron handler, and background refresh", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		opts := newTestOpts()

		(&PassthroughServicePipeline[*durationpb.Duration]{
			ID:    "service-cfg",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "service.cfg"},
			},
		}).Register(opts.Registry)

		mod := NewModule(opts)
		assert.That(t, mod.Name(), should.Equal(ModuleName))
		assert.That(t, len(mod.Dependencies()), should.Equal(3))

		outCtx, err := mod.Initialize(ctx, nil, module.HostOptions{})
		assert.NoErr(t, err)
		assert.That(t, outCtx, should.Equal(ctx))

		patterns, err := opts.Rules.ConfigPatterns(ctx)
		assert.NoErr(t, err)
		assert.That(t, len(patterns), should.Equal(1))

		assert.That(t, opts.CronDispatcher.HasHandler(CronHandlerID), should.BeTrue)
	})

	t.Run("ModuleOptions.Register parses flags", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		opts := newTestOpts()

		(&PassthroughServicePipeline[*durationpb.Duration]{
			ID:    "flag-cfg",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "flag.cfg"},
			},
		}).Register(opts.Registry)

		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		opts.Register(fs)
		assert.NoErr(t, fs.Parse([]string{
			"-cfgcache-disable-validation",
			"-cfgcache-disable-cron",
			"-cfgcache-max-ingest-memory-mb=128",
		}))

		mod := NewModule(opts)
		outCtx, err := mod.Initialize(ctx, nil, module.HostOptions{})
		assert.NoErr(t, err)
		assert.That(t, outCtx, should.Equal(ctx))
		assert.That(t, opts.Registry.MaxIngestMemoryMB, should.Equal(int64(128)))

		patterns, err := opts.Rules.ConfigPatterns(ctx)
		assert.NoErr(t, err)
		assert.That(t, len(patterns), should.Equal(0))

		// Cron handler was not registered because -cfgcache-disable-cron was set.
		opts.CronDispatcher.RegisterHandler(CronHandlerID, opts.Registry.IngestToStorage)
	})

	t.Run("Initialize propagates Registry.Initialize error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		opts := newTestOpts()

		(&PassthroughServicePipeline[*durationpb.Duration]{ID: "bad"}).Register(opts.Registry)
		badMod := NewModule(opts)
		_, err := badMod.Initialize(ctx, nil, module.HostOptions{})
		assert.ErrIsLike(t, err, "Files must not be empty")
	})
}

func TestModuleDependencies(t *testing.T) {
	t.Parallel()

	reg := &Registry{}
	(&PassthroughServicePipeline[*durationpb.Duration]{
		ID:    "dep-1",
		Files: []File{&SingleFile[*durationpb.Duration]{Path: "dep.cfg"}},
		Store: &noopStore[*durationpb.Duration]{},
	}).Register(reg)

	mod := NewModule(&ModuleOptions{Registry: reg})
	// Our noopStore added a bonus dependency over the default 2.
	assert.That(t, len(mod.Dependencies()), should.Equal(3))
}
