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

package mutate

import (
	"context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/dm/appengine/model"
)

// filterExisting removes the FwdDep objects which already exist.
//
// returns gRPC code error.
func filterExisting(c context.Context, fwdDeps []*model.FwdDep) ([]*model.FwdDep, error) {
	ret := make([]*model.FwdDep, 0, len(fwdDeps))

	err := ds.Get(c, fwdDeps)
	if err == nil {
		return nil, nil
	}

	merr, ok := err.(errors.MultiError)
	if !ok {
		return nil, err
	}

	for i, err := range merr {
		if err == nil {
			continue
		}
		ret = append(ret, fwdDeps[i])
	}

	return ret, nil
}
