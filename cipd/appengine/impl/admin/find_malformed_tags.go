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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/mapper"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	initMapper(mapperDef{
		Kind: api.MapperKind_FIND_MALFORMED_TAGS,
		Func: findMalformedTagsMapper,
		Config: mapper.JobConfig{
			Query:         mapper.Query{Kind: "InstanceTag"},
			ShardCount:    512,
			PageSize:      256, // note: 500 is a strict limit imposed by GetMulti
			TrackProgress: true,
		},
	})
}

func findMalformedTagsMapper(c context.Context, job mapper.JobID, _ *api.JobConfig, keys []*datastore.Key) error {
	return visitAndMarkTags(c, job, keys, func(t *model.Tag) string {
		if err := common.ValidateInstanceTag(t.Tag); err != nil {
			return err.Error()
		}
		return ""
	})
}
