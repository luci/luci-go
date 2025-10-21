// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	adminpb "go.chromium.org/luci/cipd/api/admin/v1"
)

func init() {
	initMapper(mapperDef{
		Kind: adminpb.MapperKind_ENUMERATE_PACKAGES,
		Func: enumPackagesMapper,
		Config: dsmapper.JobConfig{
			Query:         dsmapper.Query{Kind: "Package"},
			ShardCount:    8,
			PageSize:      256,
			TrackProgress: true,
		},
	})
}

func enumPackagesMapper(ctx context.Context, _ dsmapper.JobID, _ *adminpb.JobConfig, keys []*datastore.Key) error {
	for _, k := range keys {
		logging.Infof(ctx, "Found package: %s", k.StringID())
	}
	return nil
}
