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

// Package crbug1242998safeget enables easy to use safe Datastore Get behavior.
//
// Just import it, e.g.:
//
//     import _ go.chromium.org/luci/gae/service/datastore/crbug1242998safeget
//
// For more information, see
// https://pkg.go.dev/go.chromium.org/luci/gae/service/datastore#EnableSafeGet
package crbug1242998safeget

import (
	"go.chromium.org/luci/gae/service/datastore"
)

func init() {
	datastore.EnableSafeGet()
}
