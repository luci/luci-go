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
	"maps"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
)

// fetchConfigMeta loads stored Meta from each pipeline's Store and fetches the
// current upstream file metadata from LUCI Config.
//
// Returns a map of config.Set->file->Config(Meta only) (for any files wanted
// by any registered pipeline) for files in LUCI Config.
func (r *Registry) fetchConfigMeta(ctx context.Context, client config.Interface) (map[config.Set]map[string]config.Config, error) {
	ret := make(map[config.Set]map[string]config.Config)
	var mu sync.Mutex

	var eg errgroup.Group
	eg.SetLimit(8)

	// csToRegex will hold all our service config sets to the regex of files
	// in that set we are interested in.
	csToRegex := r.serviceFileRegexes

	if r.projectFileRegex != nil {
		// We have a projectFileRegex, meaning that we need multiple files out of
		// our project config sets. Fetch all the projects and add them to
		// csToRegex for our big fan-out below.
		projs, err := client.GetProjects(ctx)
		if err != nil {
			return nil, err
		}
		csToRegex = maps.Clone(csToRegex)
		for _, proj := range projs {
			csToRegex[config.MustProjectSet(proj.ID)] = r.projectFileRegex
		}
	} else if r.singleProjectFile != "" {
		// Otherwise, our project config set only cares about one file, so we can
		// use the slightly more optimal GetProjectConfigs.
		eg.Go(func() error {
			cfgs, err := client.GetProjectConfigs(ctx, r.singleProjectFile, true)
			if err != nil || len(cfgs) == 0 {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			for _, cfg := range cfgs {
				filesByCS := ret[cfg.ConfigSet]
				if filesByCS == nil {
					filesByCS = map[string]config.Config{}
					ret[cfg.ConfigSet] = filesByCS
				}
				filesByCS[cfg.Path] = cfg
			}
			return nil
		})
	}

	// Finally, launch all of our known-config.Set ingestConfigs.
	for cs, re := range csToRegex {
		eg.Go(func() error {
			configs, err := client.GetConfigs(ctx, cs, re.MatchString, true)
			if err := errors.Filter(err, config.ErrNoConfig); err != nil || len(configs) == 0 {
				return err
			}

			mu.Lock()
			defer mu.Unlock()
			ret[cs] = configs
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}
