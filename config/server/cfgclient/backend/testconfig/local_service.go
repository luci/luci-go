// Copyright 2016 The LUCI Authors.
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

package testconfig

import (
	"context"
	"net/url"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient/access"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
)

// WithCommonClient installs a backend.B into the supplied Context that
// loads its configuration from base.
func WithCommonClient(c context.Context, base config.Interface) context.Context {
	return backend.WithBackend(c, &client.Backend{
		Provider: &Provider{
			Base: base,
		},
	})
}

// Provider is a client.Provider that is backed by a config interface.
// This differs from client.RemoteProvider in that it takes an arbitrary
// config interface instance rather than generating a remote one.
//
// Its intended use is for testing setups.
//
// Provider performs access checks locally using access.CheckAccess.
type Provider struct {
	// Base is the base interface.
	Base config.Interface
}

var _ client.Provider = (*Provider)(nil)

// GetServiceURL implements ClientProvider.
func (p *Provider) GetServiceURL() url.URL {
	return url.URL{
		Scheme: "test",
		Host:   "example.com",
	}
}

// GetConfigClient implements ClientProvider.
func (p *Provider) GetConfigClient(c context.Context, a backend.Authority) config.Interface {
	return &boundLocalInterface{
		Context:   c,
		Interface: p.Base,
		lsp:       p,
		a:         a,
	}
}

// boundLocalInterface is a config.Interface implementation that adds ACL
// pruning to the results based on the bound authority.
type boundLocalInterface struct {
	context.Context

	// Interface is the config interface implementation.
	config.Interface

	lsp *Provider
	a   backend.Authority
}

func (bli *boundLocalInterface) GetConfig(ctx context.Context, configSet config.Set, path string, hashOnly bool) (*config.Config, error) {
	if err := access.Check(bli, bli.a, configSet); err != nil {
		return nil, config.ErrNoConfig
	}
	return bli.Interface.GetConfig(ctx, configSet, path, hashOnly)
}

func (bli *boundLocalInterface) GetProjectConfigs(ctx context.Context, path string, hashesOnly bool) ([]config.Config, error) {
	cfgs, err := bli.Interface.GetProjectConfigs(ctx, path, hashesOnly)
	if err != nil {
		return nil, err
	}
	return bli.pruneConfigList(cfgs), nil
}

func (bli *boundLocalInterface) GetRefConfigs(ctx context.Context, path string, hashesOnly bool) ([]config.Config, error) {
	cfgs, err := bli.Interface.GetRefConfigs(ctx, path, hashesOnly)
	if err != nil {
		return nil, err
	}
	return bli.pruneConfigList(cfgs), nil
}

func (bli *boundLocalInterface) GetConfigSetLocation(ctx context.Context, configSet config.Set) (*url.URL, error) {
	if err := access.Check(ctx, bli.a, configSet); err != nil {
		return nil, config.ErrNoConfig
	}
	return bli.Interface.GetConfigSetLocation(ctx, configSet)
}

func (bli *boundLocalInterface) pruneConfigList(cl []config.Config) []config.Config {
	ptr := 0
	for i := range cl {
		if err := access.Check(bli, bli.a, config.Set(cl[i].ConfigSet)); err == nil {
			cl[ptr] = cl[i]
			ptr++
		}
	}
	return cl[:ptr]
}
