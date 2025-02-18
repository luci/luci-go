// Copyright 2025 The LUCI Authors.
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

// Package gaememcache provides very minimal memcache-like functionality based
// on the legacy GAE memcache service.
//
// Using this module while running on GAE will install GAE memcache as the
// global cache provider as used by go.chromium.org/luci/server/caching library.
// This allows using GAE memcache as the cache implementation for various parts
// of the LUCI server (primary for caching various tokens and results of token
// checks).
//
// Requires `app_engine_apis: true` to be specified in the app.yaml.
//
// This package is not a full replacement for GAE memcache API, nor it is
// compatible with GAE memcache data stored through other libraries (it
// intentionally namespaces all keys it is using).
package gaememcache

import (
	"context"
	"flag"
	"fmt"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/gaememcache")

// ModuleOptions contain configuration of the gaememcache server module.
type ModuleOptions struct {
	// Nothing for now.
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	// Nothing for now.
}

// NewModule returns a server module that installs GAE memcache as the default
// global cache as used by go.chromium.org/luci/server/caching library.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &gaememcacheModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// gaememcacheModule implements module.Module.
type gaememcacheModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*gaememcacheModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*gaememcacheModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *gaememcacheModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if !opts.Prod {
		return ctx, nil
	}
	if opts.Serverless != module.GAE {
		return nil, errors.Reason("gaememcache can only be used when running on Appengine with `app_engine_apis: true`").Err()
	}
	return caching.WithGlobalCache(ctx, func(namespace string) caching.BlobCache {
		return &gaeBlobCache{namespace: fmt.Sprintf("luci.blobcache.%s", namespace)}
	}), nil
}
