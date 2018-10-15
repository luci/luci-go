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

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/mapper"
	"go.chromium.org/luci/common/errors"

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
	Kind   api.MapperKind // also used to derive mapper.ID
	Func   func(context.Context, *api.JobConfig, []*datastore.Key) error
	Config mapper.JobConfig // note: Params will be overwritten
}

// mapperID returns an identifier for this mapper (to use cross-process).
func (m *mapperDef) mapperID() mapper.ID {
	return mapper.ID(fmt.Sprintf("cipd:v1:%s", m.Kind))
}

// newMapper creates new instance of a mapping function.
func (m *mapperDef) newMapper(c context.Context, j *mapper.Job, shardIdx int) (mapper.Mapper, error) {
	cfg := &api.JobConfig{}
	if err := proto.Unmarshal(j.Config.Params, cfg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal JobConfig").Err()
	}
	return func(c context.Context, keys []*datastore.Key) error {
		return m.Func(c, cfg, keys)
	}, nil
}
