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

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
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

	// DisableValidation, if true, causes the module initialization to skip
	// registration of config validation rules with [validation.Rules].
	//
	// You can use this for backend deployments which won't serve validation
	// requests.
	DisableValidation bool
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.BoolVar(&o.DisableValidation, "cfgcache-disable-validation", false,
		"Disables registration of config validation rules.")
}

// NewModule returns a server module that registers cfgcache validation rules.
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
func (*serverModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.RequiredDependency(cfgmodule.ModuleName),
		module.OptionalDependency(cron.ModuleName),
	}
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	reg := m.registry()

	if m.vars() != cmp.Or(m.rules().Vars, &vars.Vars) {
		panic("ModuleOptions: Vars != Rules.Vars")
	}

	if err := reg.Initialize(ctx, m.vars()); err != nil {
		return nil, err
	}

	if !m.opts.DisableValidation {
		reg.InstallValidationRules(m.rules())
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
