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

package engine

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TODO(vadimsh): This is temporary dumb logic until v2 is fully implemented.
// This design most likely will change.

// requestFromTriggers examines zero or more triggers and derives a request for
// new invocation based on them.
//
// Currently picks the values from most recent trigger and disregards properties
// of the rest of them.
func requestFromTriggers(c context.Context, t []*internal.Trigger) task.Request {
	// TODO(vadimsh): Constructs Properties and Tags based on the most recent
	// trigger.
	return task.Request{
		IncomingTriggers: t,
	}
}

// putRequestIntoInv copies task.Request fields into Invocation.
//
// See getRequestFromInv for reverse.
func putRequestIntoInv(inv *Invocation, req *task.Request) {
	// TODO(vadimsh): Propagate req.Properties and req.Tags.
	inv.TriggeredBy = req.TriggeredBy
	inv.IncomingTriggersRaw = marshalTriggersList(req.IncomingTriggers)
}

// getRequestFromInv constructs task.Request from Invocation fields.
//
// It is reverse of putRequestIntoInv. Fails only on deserialization errors,
// this should be rare.
func getRequestFromInv(inv *Invocation) (task.Request, error) {
	triggers, err := inv.IncomingTriggers()
	if err != nil {
		return task.Request{}, err
	}
	return task.Request{
		TriggeredBy:      inv.TriggeredBy,
		IncomingTriggers: triggers,
	}, nil
}
