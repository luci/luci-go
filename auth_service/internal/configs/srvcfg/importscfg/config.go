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
	"errors"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/configspb"
)

var cachedImportsCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "imports.cfg",
	Type: (*configspb.GroupImporterConfig)(nil),
})

// Get returns the config stored in the datastore or a default empty one.
func Get(ctx context.Context) (*configspb.GroupImporterConfig, *config.Meta, error) {
	meta := &config.Meta{}
	cfg, err := cachedImportsCfg.Fetch(ctx, meta)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return &configspb.GroupImporterConfig{}, &config.Meta{}, nil
		}
		return nil, nil, err
	}
	return cfg.(*configspb.GroupImporterConfig), meta, nil
}

// SetInTest replaces the config for tests.
func SetInTest(ctx context.Context, cfg *configspb.GroupImporterConfig, meta *config.Meta) error {
	return cachedImportsCfg.Set(ctx, cfg, meta)
}

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) (*config.Meta, error) {
	meta := &config.Meta{}
	_, err := cachedImportsCfg.Update(ctx, meta)
	return meta, err
}

// getAuthorizedUploaders returns a map of authorized uploaders, where
// * each key is an account; and
// * each value is the set of tarballs the account is authorized to upload.
func getAuthorizedUploaders(cfg *configspb.GroupImporterConfig) map[string]stringset.Set {
	uploaders := make(map[string]stringset.Set)
	for _, entry := range cfg.GetTarballUpload() {
		for _, u := range entry.GetAuthorizedUploader() {
			if _, ok := uploaders[u]; !ok {
				uploaders[u] = stringset.New(1)
			}
			uploaders[u].Add(entry.GetName())
		}
	}
	return uploaders
}

// IsAuthorizedUploader returns whether the email is authorized to upload the
// specified tarball.
func IsAuthorizedUploader(cfg *configspb.GroupImporterConfig, email, tarballName string) bool {
	uploaders := getAuthorizedUploaders(cfg)
	authorizedTarballs, ok := uploaders[email]
	return ok && authorizedTarballs.Has(tarballName)
}
