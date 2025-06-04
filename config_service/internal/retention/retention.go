// Copyright 2023 The LUCI Authors.
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

// Package retention contains logic to clean up configs longer than the retention time.
package retention

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
)

const (
	// configRetention is the retention time for non-latest config File entities.
	configRetention = 5 * time.Hour
)

// DeleteStaleConfigs deletes stale configs which exceeds the retention time.
func DeleteStaleConfigs(ctx context.Context) error {
	var cfgsets []*model.ConfigSet
	cfgQuery := datastore.NewQuery(model.ConfigSetKind).Project("latest_revision.id")
	if err := datastore.GetAll(ctx, cfgQuery, &cfgsets); err != nil {
		return errors.Fmt("failed to query all config sets: %w", err)
	}
	cfgsetToRev := make(map[string]string, len(cfgsets))
	for _, cs := range cfgsets {
		cfgsetToRev[string(cs.ID)] = cs.LatestRevision.ID
	}

	var files []*model.File
	fileQuery := datastore.NewQuery(model.FileKind)
	if err := datastore.GetAll(ctx, fileQuery, &files); err != nil {
		return errors.Fmt("failed to query all config files: %w", err)
	}

	var toDel []*model.File
	toDelGsFiles := make(stringset.Set, len(files))
	GsFilesInUse := make(stringset.Set, len(files))
	for _, f := range files {
		if cfgsetToRev[f.Revision.Root().StringID()] == f.Revision.StringID() {
			GsFilesInUse.Add(string(f.GcsURI))
			continue
		}
		if f.CreateTime.Before(clock.Now(ctx).Add(-configRetention)) {
			toDel = append(toDel, f)
			toDelGsFiles.Add(string(f.GcsURI))
		}
	}
	// GS file's name consist of sha256 value, so they may be referred by multiple
	// File entities. Don't delete the GS files which are referred by latest
	// revision File entities.
	toDelGsFiles.DelAll(GsFilesInUse.ToSlice())

	logging.Infof(ctx, "Deleting %d stale config files", len(toDel))
	if err := datastore.Delete(ctx, toDel); err != nil {
		return errors.Fmt("failed to delete File entities: %w", err)
	}

	// Best effort to delete GCS files. No need to put into the same transaction
	// with the above File entities deletion.
	err := parallel.WorkPool(8, func(workCh chan<- func() error) {
		for _, f := range toDelGsFiles.ToSlice() {
			f := gs.Path(f)
			if !f.IsFullPath() {
				continue
			}
			workCh <- func() error {
				err := clients.GetGsClient(ctx).Delete(ctx, f.Bucket(), f.Filename())
				return errors.WrapIf(err, "cannot delete GCS file %q", f)
			}
		}
	})
	return err
}
