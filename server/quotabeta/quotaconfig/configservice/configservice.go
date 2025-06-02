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

// Package configservice provides an implementation of quotaconfig.Interface
// which fetches *pb.Policy configs stored with the LUCI Config service. Create
// an instance suitable for use with the quota library using New.
package configservice

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/server/caching"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
)

// Ensure configService implements Interface at compile-time.
var _ quotaconfig.Interface = &configService{}

// configService fetches known *pb.Policy protos from the config service.
// Implements Interface. Safe for concurrent use as long as the cache is
// also safe for concurrent use.
type configService struct {
	cache  caching.BlobCache
	cfgSet config.Set
	path   string
}

// New returns a quotaconfig.Interface which fetches known *pb.Policy protos at
// the given path for the given config set known to the LUCI Config service.
// Suitable for use with the quota library by calling
// quota.WithConfig(ctx, configservice.New(ctx, cfgSet, path)).
// Panics if caching.GlobalCache() returns nil, which shouldn't happen if the
// redisconn module (required by the quota library) has been initialized.
func New(ctx context.Context, cfgSet config.Set, path string) quotaconfig.Interface {
	cache := caching.GlobalCache(ctx, "quota.configservice")
	if cache == nil {
		// Shouldn't happen, since the quota library depends on redisconn which
		// installs a redis-based cache into the context on module initialization.
		panic(errors.New("no global cache available"))
	}
	return &configService{
		cache:  cache,
		cfgSet: cfgSet,
		path:   path,
	}
}

// Get returns a cached copy of the named *pb.Policy if it exists, or
// quotaconfig.ErrNotFound if it doesn't. Errs if the context doesn't contain a
// cfgclient.Interface (usually installed by config/service/cfgmodule).
func (c *configService) Get(ctx context.Context, name string) (*pb.Policy, error) {
	b, err := c.cache.Get(ctx, name)
	switch {
	case err == caching.ErrCacheMiss:
		return nil, quotaconfig.ErrNotFound
	case err != nil:
		return nil, errors.Fmt("retrieving cached policy %q: %w", name, err)
	}
	p := &pb.Policy{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, errors.Fmt("unmarshalling cached policy %q: %w", name, err)
	}
	return p, nil
}

// Refresh fetches all *pb.Policies from the LUCI Config service.
func (c *configService) Refresh(ctx context.Context) error {
	s := &pb.Config{}
	if err := cfgclient.Get(ctx, c.cfgSet, c.path, cfgclient.ProtoText(s), nil); err != nil {
		return errors.Fmt("fetching policy config %q for config set %q: %w", c.path, c.cfgSet, err)
	}
	v := &validation.Context{
		Context: ctx,
	}
	for i, p := range s.GetPolicy() {
		v.Enter("policy %d", i)
		quotaconfig.ValidatePolicy(v, p)
		v.Exit()
	}
	if err := v.Finalize(); err != nil {
		return errors.Fmt("policy config %q for config set %q did not pass validation: %w", c.path, c.cfgSet, err)
	}
	for _, p := range s.GetPolicy() {
		b, err := proto.Marshal(p)
		if err != nil {
			return errors.Fmt("marshalling policy %q: %w", p.Name, err)
		}
		if err := c.cache.Set(ctx, p.Name, b, 0); err != nil {
			return errors.Fmt("caching policy %q: %w", p.Name, err)
		}
	}
	return nil
}
