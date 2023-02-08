// Copyright 2022 The LUCI Authors.
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

package rpcquota

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server/module"
	quota "go.chromium.org/luci/server/quotabeta"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/quotabeta/quotaconfig/configservice"
	"go.chromium.org/luci/server/redisconn"
)

var quotaTrackOnlyKey = "go.chromium.org/luci/resultdb/internal/rpcquota:quotaModeKey"

var ModuleName = module.RegisterName("go.chromium.org/luci/resultdb/internal/rpcquota")

var _ module.Module = &rpcquotaModule{}

// rpcquotaModule implements module.Module.
type rpcquotaModule struct {
	opts *ModuleOptions
}

const (
	// Disabled: Quota is fully off.  No quotaconfig loaded, no quota
	// amounts deducted in redis, no UnaryServerInterceptor registered.
	RPCQuotaModeDisabled = "disabled"

	// Track only: deduct quota but don't fail requests.  Intended for dark
	// launch.
	RPCQuotaModeTrackOnly = "track-only"

	// RPC quota fully enabled.  Requests that exceed quota will be failed.
	RPCQuotaModeEnforce = "enforce"
)

type ModuleOptions struct {
	// RPCQuotaMode determines whether quota is off, active (but not
	// enforced), or active and enforced.
	RPCQuotaMode string
}

func (o *ModuleOptions) Register(fs *flag.FlagSet) {
	if o.RPCQuotaMode == "" {
		o.RPCQuotaMode = RPCQuotaModeDisabled
	}
	fs.Var(
		luciflag.NewChoice(&o.RPCQuotaMode, RPCQuotaModeDisabled, RPCQuotaModeTrackOnly, RPCQuotaModeEnforce),
		"rpcquota-mode",
		"RPC quota mode. Options are: disabled, track-only, enforce.")
}

// NewModule returns a module.Module for the rpcquota library initialized from
// the given *ModuleOptions.
func NewModule(opts *ModuleOptions) module.Module {
	return &rpcquotaModule{
		opts: opts,
	}
}

// NewModuleFromFlags returns a module.Module for the rpcquota library which
// can be initialized from command line flags.
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// Dependencies returns required and optional dependencies for this module.
// Implements module.Module.
func (*rpcquotaModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.RequiredDependency(redisconn.ModuleName),
		// NOTE: cfgmodule and rpcquota must be initialized before
		// quota, so it declaring quota.ModuleName as a dependency
		// doesn't work.  Instead rpcquota users must pass rpcquota
		// directly to MainWithModules to ensure it gets initialized
		// first.
		module.RequiredDependency(cfgmodule.ModuleName),
	}
}

// Name returns the module.Name for this module.
// Implements module.Module.
func (*rpcquotaModule) Name() module.Name {
	return ModuleName
}

var ErrInvalidOptions = errors.New("Invalid options")

// Initialize initializes this module by adding a quota.Use implementation to
// the serving context, according to the RPCQuotaMode in the ModuleOptions.
func (m *rpcquotaModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	switch m.opts.RPCQuotaMode {
	case RPCQuotaModeDisabled:
		// Install a stub config so that luci/server/quota Initialize
		// does not panic, but otherwise there's nothing to do.
		cfg, err := quotaconfig.NewMemory(ctx, nil)
		if err != nil {
			return nil, err
		}
		return quota.Use(ctx, cfg), nil
	case RPCQuotaModeTrackOnly:
		// Leave a flag in the context that will be checked by
		// UpdateUserQuota before returning ErrInsufficientQuota.
		// Note that this is not the common path: we expect that
		//  1. nearly all requests will be within quota
		//  2. this option is only set during the "dark" launch phase.
		ctx = context.WithValue(ctx, &quotaTrackOnlyKey, true)
	case RPCQuotaModeEnforce:
	default:
		return nil, ErrInvalidOptions
	}
	if !opts.Prod {
		logging.Warningf(ctx, "RPC quota not disabled, but not running in prod.")
	}

	cfgsvc := configservice.New(ctx, config.ServiceSet("luci-resultdb"), "rpcquota.cfg")
	if err := cfgsvc.Refresh(ctx); err != nil {
		return nil, err
	}
	ctx = quota.Use(ctx, cfgsvc)

	host.RegisterUnaryServerInterceptors(quotaCheckInterceptor())
	return ctx, nil
}
