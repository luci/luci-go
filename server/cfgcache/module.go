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
	"cmp"
	"context"
	"flag"
	"fmt"

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
)

// CronHandlerID is the server/cron handler ID registered by this module.
const CronHandlerID = "cfgcache"

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/cfgcache")

// Default is the default [Registry] installed into the server when using
// [NewModule] or [NewModuleFromFlags].
//
// If you use this, you still need to register your pipelines on this
// explicitly.
var Default Registry

// ModuleOptions contains configuration of the cfgcache server module.
type ModuleOptions struct {
	// Registry is the registry to use.
	//
	// Default is the global [Default] Registry.
	Registry *Registry

	// Vars is the varset to use when initializing the Registry.
	//
	// Default is [vars.Vars].
	Vars *vars.VarSet

	// Rules is the validation RuleSet to use when installing validation handlers.
	//
	// Default is [validation.Rules].
	// NOTE: The default RuleSet hard-codes the global VarSet internally. If you
	// want to replace `Rules` then you should replace `Vars` as well.
	Rules *validation.RuleSet

	// CronDispatcher is the cron Dispatcher to use when installing cron handlers.
	//
	// Default is [cron.Default].
	CronDispatcher *cron.Dispatcher

	// DisableValidation, if true, causes the module initialization to skip
	// registration of config validation rules with [validation.Rules].
	//
	// You can use this for backend deployments which won't serve validation
	// requests.
	DisableValidation bool

	// DisableCron, if true, causes the module initialization to skip
	// registration of the "cfgcache" endpoint with [cron.RegisterHandler].
	//
	// You can use this for frontend deployments which won't serve cron.
	DisableCron bool

	// MaxIngestMemoryMB is the maximum total memory (in MiB) of raw config
	// snapshots to ingest concurrently in the cron handler.
	//
	// If <= 0, defaults to 64.
	MaxIngestMemoryMB int64
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.BoolVar(&o.DisableValidation, "cfgcache-disable-validation", false,
		"Disables registration of config validation rules.")
	f.BoolVar(&o.DisableCron, "cfgcache-disable-cron", false,
		fmt.Sprintf("Disables the %q cron handler.", CronHandlerID))
	f.Int64Var(&o.MaxIngestMemoryMB, "cfgcache-max-ingest-memory-mb", defaultMaxIngestMemoryMB,
		"Maximum total memory (in MiB) of raw config snapshots to ingest concurrently in the cron handler.")
}

// NewModule returns a server module that registers cfgcache validation rules
// and the [CronHandlerID] cron handler.
//
// `reg` is required! Remember to explicitly register all your
// [ServicePipeline] and [ProjectPipeline] configs with `reg` before
// `server.Main` initializes modules.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &serverModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options
// through command line flags.
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
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*serverModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
//
// This freezes registration of new pipelines with the Registry (but does not
// fully initialize it yet, that will happen in Initialize).
func (m *serverModule) Dependencies() []module.Dependency {
	reg := m.registry()
	reg.finishRegistration()

	ret := make([]module.Dependency, 0, 2+len(reg.ingestConfigs))
	seen := map[module.Dependency]bool{}

	add := func(dep module.Dependency) {
		if seen[dep] {
			return
		}
		seen[dep] = true
		ret = append(ret, dep)
	}

	add(module.RequiredDependency(cfgmodule.ModuleName))
	if !m.opts.DisableCron {
		add(module.RequiredDependency(cron.ModuleName))
	}
	for _, ic := range reg.ingestConfigs {
		for _, dep := range ic.store.ModuleDependencies() {
			add(dep)
		}
	}

	return ret
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	reg := m.registry()
	reg.MaxIngestMemoryMB = cmp.Or(m.opts.MaxIngestMemoryMB, reg.MaxIngestMemoryMB, int64(defaultMaxIngestMemoryMB))

	if m.vars() != cmp.Or(m.rules().Vars, &vars.Vars) {
		panic("ModuleOptions: Vars != Rules.Vars")
	}

	if err := reg.Initialize(ctx, m.vars()); err != nil {
		return nil, err
	}

	if !m.opts.DisableValidation {
		reg.InstallValidationRules(m.rules())
	}
	if !m.opts.DisableCron {
		m.cron().RegisterHandler(CronHandlerID, reg.IngestToStorage)
	}
	return ctx, nil
}

func (m *serverModule) registry() *Registry {
	return cmp.Or(m.opts.Registry, &Default)
}

func (m *serverModule) rules() *validation.RuleSet {
	return cmp.Or(m.opts.Rules, &validation.Rules)
}

func (m *serverModule) vars() *vars.VarSet {
	return cmp.Or(m.opts.Vars, &vars.Vars)
}

func (m *serverModule) cron() *cron.Dispatcher {
	return cmp.Or(m.opts.CronDispatcher, &cron.Default)
}
