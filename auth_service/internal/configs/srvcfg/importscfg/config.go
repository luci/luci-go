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

package importscfg

import (
	"context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"

	"go.chromium.org/luci/auth_service/api/configspb"
)

var cachedImportsCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "imports.cfg",
	Type: (*configspb.GroupImporterConfig)(nil),
})

// Get returns the config stored in context.
func Get(ctx context.Context) (*configspb.GroupImporterConfig, error) {
	cfg, err := cachedImportsCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cfg.(*configspb.GroupImporterConfig), nil
}

// GetWithMetadata returns the config and its metadata stored in context.
func GetWithMetadata(ctx context.Context) (*configspb.GroupImporterConfig, *config.Meta, error) {
	meta := &config.Meta{}
	cfg, err := cachedImportsCfg.Get(ctx, meta)
	if err != nil {
		return nil, nil, err
	}
	return cfg.(*configspb.GroupImporterConfig), meta, nil
}

// SetConfig installs the cfg into the context ctx.
func SetConfig(ctx context.Context, cfg *configspb.GroupImporterConfig) error {
	return SetConfigWithMetadata(ctx, cfg, &config.Meta{})
}

// SetConfigWithMetadata installs the cfg with the given metadata into the
// context ctx.
func SetConfigWithMetadata(ctx context.Context, cfg *configspb.GroupImporterConfig, meta *config.Meta) error {
	return cachedImportsCfg.Set(ctx, cfg, meta)
}

// Update fetches the config and puts it into the datastore.
//
// It is then used by all requests that go through Middleware.
func Update(ctx context.Context) (*config.Meta, error) {
	meta := &config.Meta{}
	_, err := cachedImportsCfg.Update(ctx, meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// getAuthorizedUploaders returns a map of authorized uploaders, where
// * each key is an account; and
// * each value is the set of tarballs the account is authorized to upload.
func getAuthorizedUploaders(ctx context.Context) (map[string]stringset.Set, error) {
	cfg, err := Get(ctx)
	if err != nil {
		return map[string]stringset.Set{}, err
	}

	uploaders := make(map[string]stringset.Set)
	for _, entry := range cfg.GetTarballUpload() {
		for _, u := range entry.GetAuthorizedUploader() {
			if _, ok := uploaders[u]; !ok {
				uploaders[u] = stringset.New(1)
			}
			uploaders[u].Add(entry.GetName())
		}
	}

	return uploaders, nil
}

// IsAuthorizedUploader returns whether the email is authorized to upload the
// specified tarball.
func IsAuthorizedUploader(ctx context.Context, email, tarballName string) (bool, error) {
	uploaders, err := getAuthorizedUploaders(ctx)
	if err != nil {
		return false, err
	}

	authorizedTarballs, ok := uploaders[email]
	if !ok || !authorizedTarballs.Has(tarballName) {
		return false, nil
	}

	return true, nil
}
