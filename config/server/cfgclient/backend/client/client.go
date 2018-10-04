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

// Package client implements a config client backend for a configuration client.
package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/server/auth"
)

// Provider returns a config.Interface for the supplied parameters.
type Provider interface {
	GetServiceURL() url.URL
	GetConfigClient(context.Context, backend.Authority) config.Interface
}

// Backend returns a backend.B implementation that falls through to the supplied
// configuration service client's config.Interface, supplied by the Provider.
//
// url is the base URL to the configuration service, e.g.,
// https://example.appspot.com.
type Backend struct {
	Provider Provider
}

var _ backend.B = (*Backend)(nil)

// ServiceURL implements backend.B.
func (be *Backend) ServiceURL(c context.Context) url.URL { return be.Provider.GetServiceURL() }

// Get implements backend.B.
func (be *Backend) Get(c context.Context, configSet config.Set, path string, p backend.Params) (*config.Config, error) {
	svc := be.GetConfigInterface(c, p.Authority)

	cfg, err := svc.GetConfig(c, configSet, path, !p.Content)
	if err != nil {
		return nil, translateConfigErr(err)
	}

	return makeItem(cfg), nil
}

// GetAll implements backend.B.
func (be *Backend) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*config.Config, error) {

	svc := be.GetConfigInterface(c, p.Authority)

	var fn func(context.Context, string, bool) ([]config.Config, error)
	switch t {
	case backend.GetAllProject:
		fn = svc.GetProjectConfigs
	case backend.GetAllRef:
		fn = svc.GetRefConfigs
	default:
		return nil, errors.Reason("unknown GetAllType: %q", t).Err()
	}

	cfgs, err := fn(c, path, !p.Content)
	if err != nil || len(cfgs) == 0 {
		return nil, translateConfigErr(err)
	}

	items := make([]*config.Config, len(cfgs))
	for i := range cfgs {
		items[i] = makeItem(&cfgs[i])
	}
	return items, nil
}

// GetConfigInterface implements backend.B.
func (be *Backend) GetConfigInterface(c context.Context, a backend.Authority) config.Interface {
	return be.Provider.GetConfigClient(c, a)
}

// RemoteProvider is a Provider implementation that binds to
// a remote configuration service.
type RemoteProvider struct {
	// Host is the base host name of the configuration service, e.g.,
	// "example.appspot.com".
	Host string
	// Insecure is true if the connection should use HTTP instead of HTTPS.
	Insecure bool

	cacheLock sync.RWMutex
	cache     map[backend.Authority]config.Interface

	// testUserDelegationToken, if not nil, is the delegation token to use for
	// AsUser calls. This is done to mock delegation token generation.
	testUserDelegationToken string
}

var _ Provider = (*RemoteProvider)(nil)

// GetServiceURL implements Provider.
func (p *RemoteProvider) GetServiceURL() url.URL {
	u := url.URL{
		Scheme: "https",
		Host:   p.Host,
		Path:   "/_ah/api/config/v1/",
	}
	if p.Insecure {
		u.Scheme = "http"
	}
	return u
}

// GetConfigClient implements Provider.
func (p *RemoteProvider) GetConfigClient(c context.Context, a backend.Authority) config.Interface {
	p.cacheLock.RLock()
	impl, ok := p.cache[a]
	p.cacheLock.RUnlock()
	if ok {
		return impl
	}

	p.cacheLock.Lock()
	defer p.cacheLock.Unlock()

	if impl, ok := p.cache[a]; ok {
		return impl
	}

	// Create our remote implementation.
	impl = remote.New(p.Host, p.Insecure, func(c context.Context) (*http.Client, error) {
		var opts []auth.RPCOption
		if a == backend.AsUser && p.testUserDelegationToken != "" {
			opts = append(opts, auth.WithDelegationToken(p.testUserDelegationToken))
		}
		t, err := auth.GetRPCTransport(c, rpcAuthorityKind(a), opts...)
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	})
	if p.cache == nil {
		p.cache = make(map[backend.Authority]config.Interface, 3)
	}
	p.cache[a] = impl

	return impl
}

func translateConfigErr(err error) error {
	switch err {
	case config.ErrNoConfig:
		return config.ErrNoConfig
	default:
		return err
	}
}

func makeItem(cfg *config.Config) *config.Config {
	return &config.Config{
		Meta: config.Meta{
			ConfigSet:   cfg.ConfigSet,
			Path:        cfg.Path,
			ContentHash: cfg.ContentHash,
			Revision:    cfg.Revision,
			ViewURL:     cfg.ViewURL,
		},
		Content: cfg.Content,
	}
}

// rpcAuthorityKind returns the RPC authority associated with this authority
// level.
func rpcAuthorityKind(a backend.Authority) auth.RPCAuthorityKind {
	switch a {
	case backend.AsAnonymous:
		return auth.NoAuth
	case backend.AsService:
		return auth.AsSelf
	case backend.AsUser:
		return auth.AsUser
	default:
		panic(fmt.Errorf("unknown config Authority (%d)", a))
	}
}
