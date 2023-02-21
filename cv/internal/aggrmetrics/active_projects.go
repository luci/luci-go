// Copyright 2021 The LUCI Authors.
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

package aggrmetrics

import (
	"context"
	"encoding/json"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
)

const (
	activeProjectsTTL = 20 * time.Minute
	activeProjectsKey = "v1"
)

// activeProjects returns LUCI projects currently active in CV.
func activeProjects(ctx context.Context) ([]string, error) {
	out, err := activeProjectsCache.GetOrCreate(ctx, activeProjectsKey, func() (any, time.Duration, error) {
		out, err := prjcfg.GetAllProjectIDs(ctx, true /* enabled only*/)
		return out, activeProjectsTTL, err
	})
	if err != nil {
		return nil, err
	}
	return stringset.NewFromSlice(out.([]string)...).ToSortedSlice(), nil
}

// activeProjectsCache caches active projects to avoid hammering Datastore on
// something which doesn't change very often.
var activeProjectsCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(1),
	GlobalNamespace: "aggrmetrics_active_projects",
	Marshal:         json.Marshal,
	Unmarshal: func(blob []byte) (any, error) {
		// There are ~100 active LUCI projects.
		out := make([]string, 0, 100)
		err := json.Unmarshal(blob, &out)
		return out, err
	},
}
