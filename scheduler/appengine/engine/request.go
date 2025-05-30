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
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils"
)

// putRequestIntoInv copies task.Request fields into Invocation.
//
// See getRequestFromInv for reverse. Fails only on serialization errors,
// this should be rare.
func putRequestIntoInv(inv *Invocation, req *task.Request) error {
	var props []byte
	if req.Properties != nil {
		var err error
		if props, err = proto.Marshal(req.Properties); err != nil {
			return errors.Fmt("can't marshal Properties: %w", err)
		}
	}

	if err := utils.ValidateKVList("tag", req.Tags, ':'); err != nil {
		return errors.Fmt("rejecting bad tags: %w", err)
	}
	sortedTags := append([]string(nil), req.Tags...)
	sort.Strings(sortedTags)

	// Note: DebugLog is handled in a special way by initInvocation.
	inv.TriggeredBy = req.TriggeredBy
	inv.IncomingTriggersRaw = marshalTriggersList(req.IncomingTriggers)
	inv.PropertiesRaw = props
	inv.Tags = sortedTags
	return nil
}

// getRequestFromInv constructs task.Request from Invocation fields.
//
// It is reverse of putRequestIntoInv. Fails only on deserialization errors,
// this should be rare.
func getRequestFromInv(inv *Invocation) (r task.Request, err error) {
	r.TriggeredBy = inv.TriggeredBy
	if r.IncomingTriggers, err = inv.IncomingTriggers(); err != nil {
		return r, errors.Fmt("failed to deserialize incoming triggers: %w", err)
	}
	if len(inv.PropertiesRaw) != 0 {
		props := &structpb.Struct{}
		if err = proto.Unmarshal(inv.PropertiesRaw, props); err != nil {
			return r, errors.Fmt("failed to deserialize properties: %w", err)
		}
		r.Properties = props
	}
	r.Tags = inv.Tags
	return
}
