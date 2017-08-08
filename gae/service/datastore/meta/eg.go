// Copyright 2015 The LUCI Authors.
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

package meta

import (
	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

// EntityGroupMeta is the model corresponding to the __entity_group__ model in
// appengine. You shouldn't need to use this struct directly, but instead should
// use GetEntityGroupVersion.
type EntityGroupMeta struct {
	kind   string  `gae:"$kind,__entity_group__"`
	id     int64   `gae:"$id,1"`
	Parent *ds.Key `gae:"$parent"`

	Version int64 `gae:"__version__"`
}

// GetEntityGroupVersion returns the entity group version for the entity group
// containing root. If the entity group doesn't exist, this function will return
// zero and a nil error.
func GetEntityGroupVersion(c context.Context, key *ds.Key) (int64, error) {
	egm := &EntityGroupMeta{Parent: key.Root()}
	err := ds.Get(c, egm)
	ret := egm.Version
	if err == ds.ErrNoSuchEntity {
		// this is OK for callers. The version of the entity group is effectively 0
		// in this case.
		err = nil
	}
	return ret, err
}
