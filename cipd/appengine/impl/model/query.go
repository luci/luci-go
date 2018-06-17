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

package model

import (
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// SearchInstances lists instances of a package with all given tags attached.
//
// Only does a query over Instances entities. Doesn't check whether the Package
// entity exists. Returns up to pageSize entities, plus non-nil cursor (if
// there are more results). 'pageSize' must be positive.
func SearchInstances(c context.Context, pkg string, tags []*api.Tag, pageSize int32, cursor datastore.Cursor) (out []*Instance, nextCur datastore.Cursor, err error) {
	return nil, nil, fmt.Errorf("not implemented")
}
