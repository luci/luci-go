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

package run

import (
	"context"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
)

// EventboxRecipient returns eventbox.Recipient for a given Run.
func EventboxRecipient(ctx context.Context, runID common.RunID) eventbox.Recipient {
	return (*recipient)(datastore.MakeKey(ctx, RunKind, string(runID)))
}

// recipient implements eventbox.Recipient.
type recipient datastore.Key

// Key is the Datastore key of the recipient.
//
// The corresponding entity doesn't have to exist.
func (r *recipient) Key() *datastore.Key {
	return (*datastore.Key)(r)
}

// MonitoringString returns the value for the metric field "recipient".
//
// There should be very few distinct values.
func (r *recipient) MonitoringString() string {
	// There are lots of Runs, so aggregate all their metrics behind their LUCI
	// project.
	return "Run/" + common.RunID(r.Key().StringID()).LUCIProject()
}
