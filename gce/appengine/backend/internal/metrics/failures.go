// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/gce/appengine/model"
)

var (
	creationFailures = metric.NewCounter(
		"gce/failures/creation",
		"The number of failures during GCE instance creation.",
		nil,
		field.Int("code"),
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
	)
)

// UpdateFailures increments failure counters.
func UpdateFailures(c context.Context, code int, vm *model.VM) {
	creationFailures.Add(c, int64(1), code, vm.Prefix, vm.Attributes.GetProject(), vm.Attributes.GetZone())
}
