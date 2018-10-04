// Copyright 2017 The LUCI Authors.
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

package revocation

import (
	"context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/retry/transient"
)

// GenerateTokenID produces an int64 that can be used as a token identifier.
//
// We reuse datastore ID generator to produce token ids. The tokens are not
// actually stored in the datastore. The generated ID sequence is associated
// with some entity kind (indicated via 'kind'). If we ever need to restart the
// ID sequence, this kind can be changed.
func GenerateTokenID(c context.Context, kind string) (int64, error) {
	// Note: AllocateIDs modifies passed slice in place, by replacing the keys
	// there.
	keys := []*datastore.Key{
		datastore.NewKey(c, kind, "", 0, nil),
	}
	if err := datastore.AllocateIDs(c, keys); err != nil {
		return 0, transient.Tag.Apply(err)
	}
	return keys[0].IntID(), nil
}
