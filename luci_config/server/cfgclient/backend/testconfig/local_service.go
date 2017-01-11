// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testconfig

import (
	"net/url"

	ccfg "github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/access"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/client"

	"golang.org/x/net/context"
)

// WithCommonClient installs a backend.B into the supplied Context that
// loads its configuration from base.
func WithCommonClient(c context.Context, base ccfg.Interface) context.Context {
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
	Base ccfg.Interface
}

var _ client.Provider = (*Provider)(nil)

// GetConfigClient implements ClientProvider.
func (p *Provider) GetConfigClient(c context.Context, a backend.Authority) ccfg.Interface {
	return &boundLocalInterface{
		Context:   c,
		Interface: p.Base,
		lsp:       p,
		a:         a,
	}
}

// boundLocalInterface is a ccfg.Interface implementation that adds ACL
// pruning to the results based on the bound authority.
type boundLocalInterface struct {
	context.Context

	// Interface is the config interface implementation.
	ccfg.Interface

	lsp *Provider
	a   backend.Authority
}

func (bli *boundLocalInterface) GetConfig(ctx context.Context, configSet, path string, hashOnly bool) (*ccfg.Config, error) {
	if err := access.Check(bli, bli.a, cfgtypes.ConfigSet(configSet)); err != nil {
		return nil, ccfg.ErrNoConfig
	}
	return bli.Interface.GetConfig(ctx, configSet, path, hashOnly)
}

func (bli *boundLocalInterface) GetProjectConfigs(ctx context.Context, path string, hashesOnly bool) ([]ccfg.Config, error) {
	cfgs, err := bli.Interface.GetProjectConfigs(ctx, path, hashesOnly)
	if err != nil {
		return nil, err
	}
	return bli.pruneConfigList(cfgs), nil
}

func (bli *boundLocalInterface) GetRefConfigs(ctx context.Context, path string, hashesOnly bool) ([]ccfg.Config, error) {
	cfgs, err := bli.Interface.GetRefConfigs(ctx, path, hashesOnly)
	if err != nil {
		return nil, err
	}
	return bli.pruneConfigList(cfgs), nil
}

func (bli *boundLocalInterface) GetConfigSetLocation(ctx context.Context, configSet string) (*url.URL, error) {
	if err := access.Check(ctx, bli.a, cfgtypes.ConfigSet(configSet)); err != nil {
		return nil, cfgclient.ErrNoConfig
	}
	return bli.Interface.GetConfigSetLocation(ctx, configSet)
}

func (bli *boundLocalInterface) pruneConfigList(cl []ccfg.Config) []ccfg.Config {
	ptr := 0
	for i := range cl {
		if err := access.Check(bli, bli.a, cfgtypes.ConfigSet(cl[i].ConfigSet)); err == nil {
			cl[ptr] = cl[i]
			ptr++
		}
	}
	return cl[:ptr]
}
