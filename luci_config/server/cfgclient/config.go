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

package cfgclient

import (
	"net/url"

	"go.chromium.org/luci/common/config"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// ErrNoConfig is a sentinel error returned by Get when the requested
// configuration is not found.
//
// This is an alias of go.chromium.org/luci/common/config.ErrNoConfig for
// backward compatibility.
var ErrNoConfig = config.ErrNoConfig

// Meta is metadata about a single configuration file.
//
// This differs from backend.Meta in that it uses the ConfigSet type for the
// ConfigSet field.
type Meta struct {
	// ConfigSet is the item's config set.
	ConfigSet cfgtypes.ConfigSet
	// Path is the item's path within its config set.
	Path string

	// ContentHash is the content hash.
	ContentHash string
	// Revision is the revision string.
	Revision string
	// ViewURL is the URL surfaced for viewing the config.
	ViewURL string
}

// Authority is the authority on whose behalf a request is operating.
type Authority backend.Authority

var (
	// AsAnonymous requests config data as an anonymous user.
	//
	// Corresponds to auth.NoAuth.
	AsAnonymous = Authority(backend.AsAnonymous)

	// AsService requests config data as the currently-running service.
	//
	// Corresponds to auth.AsSelf.
	AsService = Authority(backend.AsService)

	// AsUser requests config data as the currently logged-in user.
	//
	// Corresponds to auth.AsUser.
	AsUser = Authority(backend.AsUser)
)

// ServiceURL returns the URL of the config service.
func ServiceURL(c context.Context) url.URL { return backend.Get(c).ServiceURL(c) }

// Get retrieves a single configuration file.
//
// r, if not nil, is a Resolver that will load the configuration data. If nil,
// the configuration data will be discarded (useful if you only care about
// metas).
//
// meta, if not nil, will have the configuration's Meta loaded into it on
// success.
func Get(c context.Context, a Authority, cs cfgtypes.ConfigSet, path string, r Resolver, meta *Meta) error {
	be := backend.Get(c)

	params := backend.Params{
		Authority: backend.Authority(a),
		Content:   r != nil,
	}

	if fr, ok := r.(FormattingResolver); ok {
		params.FormatSpec = fr.Format()
	}

	item, err := be.Get(c, string(cs), path, params)
	if err != nil {
		return err
	}

	if meta != nil {
		*meta = *makeMeta(&item.Meta)
	}
	if r == nil {
		return nil
	}
	return r.Resolve(item)
}

// Projects retrieves all named project configurations.
//
// r, if not nil, is a MultiResolver that will load the configuration data.
// If nil, the configuration data will be discarded (useful if you only care
// about metas). If the MultiResolver operates on a slice (which it probably
// will), each meta and/or error index will correspond to its slice index.
//
// If meta is not nil, it will be populated with a slice of *Meta entries
// for each loaded configuration, in the same order that r receives them. If
// r resolves to a slice, the indexes for each resolved slice entry and meta
// entry should align unless r is doing something funky.
//
// Two types of failure may happen here. A systemic failure fails to load the
// set of project configurations. This will be returned directly.
//
// A resolver failure happens when the configuratiokns load, but could not be
// resolved. In this case, any successful resolutions and "meta" will be
// populated and an errors.MultiError will be returned containing non-nil
// errors at indices whose configs failed to resolve.
func Projects(c context.Context, a Authority, path string, r MultiResolver, meta *[]*Meta) error {
	return getAll(c, a, backend.GetAllProject, path, r, meta)
}

// Refs retrieves all named ref configurations.
//
// See Projects for individual argument descriptions.
func Refs(c context.Context, a Authority, path string, r MultiResolver, meta *[]*Meta) error {
	return getAll(c, a, backend.GetAllRef, path, r, meta)
}

func getAll(c context.Context, a Authority, t backend.GetAllTarget, path string, r MultiResolver,
	meta *[]*Meta) error {

	be := backend.Get(c)

	params := backend.Params{
		Authority: backend.Authority(a),
		Content:   r != nil,
	}

	// If we're fetching content, apply a formatting specification.
	if fr, ok := r.(FormattingResolver); ok {
		params.FormatSpec = fr.Format()
	}

	items, err := be.GetAll(c, t, path, params)
	if err != nil {
		return err
	}

	// Load our metas.
	if meta != nil {
		metaSlice := *meta
		if metaSlice == nil {
			metaSlice = make([]*Meta, 0, len(items))
		} else {
			// Re-use the supplied slice.
			metaSlice = metaSlice[:0]
		}

		for _, item := range items {
			metaSlice = append(metaSlice, makeMeta(&item.Meta))
		}

		*meta = metaSlice
	}

	if r == nil {
		return nil
	}

	// Resolve our items. If any individual resolution fails, return that failure
	// as a positional error in a MultiError.
	lme := errors.NewLazyMultiError(len(items))
	r.PrepareMulti(len(items))
	for i, it := range items {
		if err := r.ResolveItemAt(i, it); err != nil {
			lme.Assign(i, err)
		}
	}
	return lme.Get()
}

func makeMeta(b *backend.Meta) *Meta {
	return &Meta{
		ConfigSet:   cfgtypes.ConfigSet(b.ConfigSet),
		Path:        b.Path,
		ContentHash: b.ContentHash,
		Revision:    b.Revision,
		ViewURL:     b.ViewURL,
	}
}
