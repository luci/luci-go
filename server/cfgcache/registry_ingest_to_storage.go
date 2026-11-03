// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"context"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
)

// loadedStoreMeta maps a pipelineID to any per-config.Set metadata for that
// pipeline.
type loadedStoreMeta map[string]map[config.Set]Meta

// IngestToStorage fetches the latest config(s) for entries from LUCI Config,
// runs Transform on changed snapshots, and updates storage.
//
// In production, this method is called by the [CronHandlerID] cron handler.
//
// Exposed for testing so unit tests can exercise fetching and transforming
// configs from an in-memory cfgclient without running a cron dispatcher.
func (r *Registry) IngestToStorage(ctx context.Context) error {
	r.checkInitialized()

	client := cfgclient.Client(ctx)

	delta, err := r.findIngestDelta(ctx, client)
	if err != nil || delta == nil {
		return err
	}

	return r.doIngest(ctx, client, delta)
}

type ingestionDelta struct {
	// Current state of configuration metadata loaded from LUCI Config.
	inConfigMeta loadedConfig

	// Current state of configuration loaded from all the ingestConfigs.
	inStoreMeta loadedStoreMeta

	// The set of ingest configs which need re-ingestion.
	//
	// The keys of the map indicate the full set of config sets we need to
	// process (services and projects).
	//
	// NOTE: The same ingestConfig may appear for multiple config.Sets (e.g.
	// project config pipelines).
	needsIngest map[config.Set][]*ingestConfig

	// Map of ingestConfig -> config sets which are no longer present/relevant in
	// LUCI Config.
	//
	// This includes things like project removals, but can also indicate that
	// some config file no longer exists within a config.Set.
	toDelete map[*ingestConfig][]config.Set
}

// findIngestDelta computes a [ingestionDelta] off of the current state of
// LUCI Config and all the ingestConfig stores.
//
// Returns `nil` if there is no work to be done.
func (r *Registry) findIngestDelta(ctx context.Context, client config.Interface) (*ingestionDelta, error) {
	eg, ctx := errgroup.WithContext(ctx)

	ret := &ingestionDelta{
		toDelete: map[*ingestConfig][]config.Set{},
	}

	eg.Go(func() (err error) {
		ret.inStoreMeta, err = r.fetchStoreMeta(ctx)
		return
	})
	eg.Go(func() (err error) {
		ret.inConfigMeta, err = r.fetchConfigMeta(ctx, client)
		return
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Next, let's compute which config sets we actually need to ingest.
	//
	// It's possible for multiple ingestConfigs to consume the same config set.
	ret.needsIngest = map[config.Set][]*ingestConfig{}

	for _, ic := range r.ingestConfigs {
		want := ic.allDesiredMeta(ret.inConfigMeta)

		// Now, filter `want` by what we already have.
		for cs, haveMeta := range ret.inStoreMeta[ic.id] {
			wantMeta, ok := want[cs]
			if !ok {
				// If we have this config set in storage, but it's not in our wanted
				// set, we can drop it from storage.
				ret.toDelete[ic] = append(ret.toDelete[ic], cs)
				continue
			}

			// Trim out any configsets from want that already match the Meta we have.
			if wantMeta.SameInput(haveMeta) {
				delete(want, cs)
			}
		}

		// Now for any remaining config.Sets which need ingestion, add `ic`.
		for cs := range want {
			ret.needsIngest[cs] = append(ret.needsIngest[cs], ic)
		}
	}

	if len(ret.toDelete) == 0 && len(ret.needsIngest) == 0 {
		return nil, nil
	}

	return ret, nil
}

// doIngest actually executes the work described in `delta`, deleting unneeded
// configs, and also pulling the full config content, transforming it, and
// storing it.
func (r *Registry) doIngest(ctx context.Context, client config.Interface, delta *ingestionDelta) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(8)

	sem := semaphore.NewWeighted(r.MaxIngestMemoryMB)

	for ic, configSets := range delta.toDelete {
		eg.Go(func() error {
			return ic.store.Delete(ctx, ic.id, configSets)
		})
	}

	for cs, needICs := range delta.needsIngest {
		matcher := compileFilesRegex(needICs)

		var expectedSize int64
		for _, cfg := range delta.inConfigMeta[cs] {
			if matcher.MatchString(cfg.Path) {
				expectedSize += cfg.ContentSize
			}
		}
		weight := r.clampedSizeMB(expectedSize)

		eg.Go(func() error {
			ctx := logging.SetField(ctx, "config.Set", cs)

			// Wait until we can process at least our expected amount of config input
			// data.
			if err := sem.Acquire(ctx, weight); err != nil {
				return err
			}
			defer sem.Release(weight)

			// Get all the configs, including their data.
			logging.Infof(ctx, "requesting configs in %q matching %q", cs, matcher.String())
			configs, err := client.GetConfigs(ctx, cs, matcher.MatchString, false)
			if err != nil || len(configs) == 0 {
				return err
			}
			flattened := flatSnapshotFromMap(configs)

			// Convert the config content from `string` to `[]byte` once for all
			// ingestConfigs. validateAndTransform will filter this by whatever each
			// ingestConfig needs when unmarshaling before passing it through to the
			// user transform function.
			rawFiles := make(map[string][]byte)
			for _, cfg := range flattened {
				rawFiles[cfg.Path] = []byte(cfg.Content)
			}

			for _, ic := range needICs {
				ctx := logging.SetField(ctx, "pipeline", ic.id)

				// Double-check the metadata; there can be skew between the first time
				// we scanned the metadata and now.
				meta, ok := ic.desiredMeta(flattened)
				if !ok {
					// This is very unlikely - it means that between the time we scanned
					// metadata and now *all* configuration files which match ic
					// disappeared from the config server.
					continue
				}
				// Check to see if `meta` now matches what we actually have stored - if
				// it does, then skip the transformation.
				if stored, ok := delta.inStoreMeta[ic.id][cs]; ok && stored.SameInput(meta) {
					continue
				}

				// Now, transform the raw file content into this ingestConfig's storage
				// format.
				valCtx := &validation.Context{Context: ctx}
				logging.Infof(ctx, "Applying TransformFunc v%d.", ic.transformVersion)
				out, err := ic.validateAndTransform(valCtx, cs, rawFiles)
				if err == nil {
					// Treat all validation errors as fatal for the purpose of ingestion.
					if valErr := valCtx.Finalize(); valErr != nil {
						err = valErr.(*validation.Error).WithSeverity(validation.Blocking)
					}
				}
				if err != nil {
					logging.Infof(ctx, "TransformFunc v%d failed: %s", ic.transformVersion, err)
					return errors.Fmt("validating/transforming %s (%s): %w", ic.id, cs, err)
				}

				// Finally, put this into the store.
				//
				// TODO: We could let these Puts operate asynchronously from our main
				// loop here, but it's extra complexity (`eg.Go` will block if the
				// group is full, so we would have to use another group, then waiting
				// for them both to finish is hard because we would have to wait for
				// them both at the same time, etc.)
				meta.ContentSize = int64(proto.Size(out))
				logging.Infof(ctx, "Storing %+v.", cs, meta)
				if err := ic.store.Put(ctx, ic.id, cs, meta, out); err != nil {
					return err
				}
			}
			return nil
		})
	}

	return eg.Wait()
}
