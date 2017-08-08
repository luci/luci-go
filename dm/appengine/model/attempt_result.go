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

package model

import (
	"go.chromium.org/gae/service/datastore"
	dm "go.chromium.org/luci/dm/api/service/v1"
)

// AttemptResult holds the raw, compressed json blob returned from the
// execution.
type AttemptResult struct {
	_id     int64          `gae:"$id,1"`
	Attempt *datastore.Key `gae:"$parent"`

	// The sizes and expirations are denormalized across Attempt and
	// AttemptResult.
	Data dm.JsonResult `gae:",noindex"`
}
