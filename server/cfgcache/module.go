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
	"math/rand"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
)

const (
	// CronHandlerID is the server/cron handler ID registered by this module.
	CronHandlerID = "cfgcache"

	// memoryRefreshID is the RunInBackground activity name for refreshing in-memory
	// configs.
	memoryRefreshID = "cfgcache"

	// StatusPagePath is the HTTP route path where the cfgcache status page is
	// mounted.
	StatusPagePath = "/admin/cfgcache"

	// StatusIngestPath is the HTTP POST route path that triggers an immediate
	// config ingestion from LUCI Config into storage.
	StatusIngestPath = "/admin/cfgcache/ingest"

	// StatusConfigPath is the HTTP GET route path that returns a stored config
	// message encoded as JSONPB (query parameters: `pipeline` and `config_set`).
	StatusConfigPath = "/admin/cfgcache/config"
)

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

	// DisableInMemory, if true, causes the module initialization to skip the
	// background refresh loop which loads all configs from storage into
	// memory.
	//
	// You can use this for backend deployments (e.g. cron) which don't need the
	// in-memory form of the config.
	//
	// If this is `true` and your service tries to read the data anyway via
	// [RegisteredProjects.GetData], [RegisteredProjects.GetAllData], or
	// [RegisteredService.GetData], they will return an error saying that the
	// data was never refreshed.
	DisableInMemory bool

	// MemoryRefreshTimeout is how long to allow one in-memory refresh to take.
	//
	// If <= 0, defaults to 1 minute.
	MemoryRefreshTimeout time.Duration

	// MaxMemoryStaleness is how long to allow in-memory config data to become
	// stale before returning errors when the service code tries to read it.
	//
	// If <= 0, defaults to 10 minutes.
	MaxMemoryStaleness time.Duration

	// MemoryRefreshEvery is the duration between refresh periods.
	//
	// If <= 0, defaults to 60s.
	MemoryRefreshEvery time.Duration

	// StatusAdminGroup is the Chrome Infra Auth group authorized to view the
	// [StatusPagePath] status page, read the full contents of all stored service
	// and project configs via [StatusConfigPath], and trigger config imports via
	// [StatusIngestPath] in production.
	//
	// Note that granting access to this group bypasses per-project LUCI Config
	// read ACLs for any [ProjectPipeline] backed by a [GettableStore].
	//
	// If this is unset (default), the status page and its sub-routes are
	// disabled.
	StatusAdminGroup string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.BoolVar(&o.DisableValidation, "cfgcache-disable-validation", false,
		"Disables registration of config validation rules.")
	f.BoolVar(&o.DisableCron, "cfgcache-disable-cron", false,
		fmt.Sprintf("Disables the %q cron handler.", CronHandlerID))
	f.Int64Var(&o.MaxIngestMemoryMB, "cfgcache-max-ingest-memory-mb", defaultMaxIngestMemoryMB,
		"Maximum total memory (in MiB) of raw config snapshots to ingest concurrently in the cron handler.")
	f.BoolVar(&o.DisableInMemory, "cfgcache-disable-in-memory", false,
		"Disables the in-memory refresh routine to load configs into memory.")
	f.DurationVar(&o.MemoryRefreshTimeout, "cfgcache-memory-refresh-timeout", time.Minute,
		"How long to allow one in-memory config refresh to take.")
	f.DurationVar(&o.MaxMemoryStaleness, "cfgcache-max-memory-staleness", 10*time.Minute,
		"How long to allow in-memory configs to be stale before returning errors.")
	f.DurationVar(&o.MemoryRefreshEvery, "cfgcache-memory-refresh-every", time.Minute,
		"How frequently to refresh in-memory configs.")
	f.StringVar(&o.StatusAdminGroup, "cfgcache-status-admin-group", "",
		fmt.Sprintf("Auth group allowed to access %q, read all stored service/project configs, and trigger imports (empty disables status page).", StatusPagePath))
}

// NewModule returns a server module that registers cfgcache validation rules,
// the [CronHandlerID] cron handler, and the in-memory refresh background loop.
//
// Remember to explicitly register all your [ServicePipeline] and
// [ProjectPipeline] configs with `opts.Registry` (or [Default]) before
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

	ret := make([]module.Dependency, 0, 3+len(reg.ingestConfigs))
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
	if m.opts.StatusAdminGroup != "" {
		add(module.RequiredDependency(secrets.ModuleName))
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
	reg.MaxIngestMemoryMB = cmp.Or(m.opts.MaxIngestMemoryMB, reg.MaxIngestMemoryMB)
	reg.MaxMemoryStaleness = cmp.Or(m.opts.MaxMemoryStaleness, reg.MaxMemoryStaleness)
	reg.MemoryRefreshTimeout = cmp.Or(m.opts.MemoryRefreshTimeout, reg.MemoryRefreshTimeout)

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
	if !m.opts.DisableInMemory {
		if m.opts.MemoryRefreshEvery <= 0 {
			m.opts.MemoryRefreshEvery = time.Minute
		}
		if err := reg.RefreshInMemory(ctx); err != nil {
			return nil, err
		}
		host.RunInBackground(memoryRefreshID, m.refreshLoop)
	}
	if m.opts.StatusAdminGroup != "" {
		m.installStatusHandlers(host, opts)
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

// refreshLoop periodically refreshes all gettable configs in the registry
// until ctx is done.
func (m *serverModule) refreshLoop(ctx context.Context) {
	reg := m.registry()

	// Randomly widen our refresh window (early or late) by 20%.
	//
	// Although we always refresh the configs right away in Initialize, this will
	// help quickly spread out instances's pings against the stored configs.
	jitterWindow := int64(float64(m.opts.MemoryRefreshEvery) * 0.2)

	for {
		jitter := time.Duration(rand.Int63n(jitterWindow))
		if r := <-clock.After(ctx, m.opts.MemoryRefreshEvery+jitter); r.Err != nil {
			return // the context is canceled
		}
		// Note that RefreshInMemory internally logs errors, so we don't need to
		// log them again here.
		_ = reg.RefreshInMemory(ctx)
	}
}
