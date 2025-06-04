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
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	api "go.chromium.org/luci/cipd/api/admin/v1"
)

// A registry of mapping job configurations.
//
// Populated during init time. See other *.go files in this package.
var mappers = map[api.MapperKind]*mapperDef{}

// initMapper is called during init time to register some mapper kind.
func initMapper(d mapperDef) {
	if _, ok := mappers[d.Kind]; ok {
		panic(fmt.Sprintf("mapper with kind %s has already been initialized", d.Kind))
	}
	mappers[d.Kind] = &d
}

// mapperDef is some single flavor of mapping jobs.
//
// It contains parameters for the mapper (what entity to map over, number of
// shards, etc), and the actual mapping function.
type mapperDef struct {
	Kind   api.MapperKind // also used to derive dsmapper.ID
	Func   func(context.Context, dsmapper.JobID, *api.JobConfig, []*datastore.Key) error
	Config dsmapper.JobConfig // note: Params will be overwritten
}

// mapperID returns an identifier for this mapper (to use cross-process).
func (m *mapperDef) mapperID() dsmapper.ID {
	return dsmapper.ID(fmt.Sprintf("cipd:v1:%s", m.Kind))
}

// newMapper creates new instance of a mapping function.
func (m *mapperDef) newMapper(ctx context.Context, j *dsmapper.Job, shardIdx int) (dsmapper.Mapper, error) {
	cfg := &api.JobConfig{}
	if err := proto.Unmarshal(j.Config.Params, cfg); err != nil {
		return nil, errors.Fmt("failed to unmarshal JobConfig: %w", err)
	}
	return func(ctx context.Context, keys []*datastore.Key) error {
		return m.Func(ctx, j.ID, cfg, keys)
	}, nil
}
