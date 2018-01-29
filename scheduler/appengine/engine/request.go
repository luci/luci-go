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
	"strings"

	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/net/context"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils"
)

// TODO(vadimsh): This is temporary dumb logic until v2 is fully implemented.
// This design most likely will change.

// TODO(vadimsh): Needs more tests. Postponed until v2, since this file will
// likely be largely rewritten.

// requestBuilder is a task.Request in a process of being prepared.
//
// This type exists mostly to group a bunch of utility methods together.
type requestBuilder struct {
	task.Request

	ctx context.Context // for logging
}

// requestFromTriggers examines zero or more triggers and derives a request for
// new invocation based on them.
//
// Currently picks the values from most recent trigger and disregards properties
// of the rest of them.
//
// TODO(vadimsh): Allow returning errors and handle them somehow? By skipping
// "broken" triggers?
func requestFromTriggers(c context.Context, t []*internal.Trigger) task.Request {
	if len(t) == 0 {
		// A force-triggered job, nowhere to derive Properties from.
		return task.Request{}
	}

	req := requestBuilder{ctx: c}

	// 't' is semantically a set. Sort triggers by the creation time, most recent
	// last, so the task manager (and UI) sees them ordered already.
	req.IncomingTriggers = append(req.IncomingTriggers, t...)
	sortTriggers(req.IncomingTriggers)

	// Derive properties and tags from the most recent trigger for now.
	newest := req.IncomingTriggers[len(req.IncomingTriggers)-1]
	switch p := newest.Payload.(type) {
	case *internal.Trigger_Noop:
		req.prepareNoopRequest(p.Noop)
	case *internal.Trigger_Gitiles:
		req.prepareGitilesRequest(p.Gitiles)
	case *internal.Trigger_Buildbucket:
		req.prepareBuildbucketRequest(p.Buildbucket)
	default:
		req.debugLog("Unrecognized trigger payload of type %T, ignoring", p)
	}

	return req.Request
}

// putRequestIntoInv copies task.Request fields into Invocation.
//
// See getRequestFromInv for reverse. Fails only on serialization errors,
// this should be rare.
func putRequestIntoInv(inv *Invocation, req *task.Request) error {
	var props []byte
	if req.Properties != nil {
		var err error
		if props, err = proto.Marshal(req.Properties); err != nil {
			return errors.Annotate(err, "can't marshal Properties").Err()
		}
	}

	if err := utils.ValidateKVList("tag", req.Tags, ':'); err != nil {
		return errors.Annotate(err, "rejecting bad tags").Err()
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
		return r, errors.Annotate(err, "failed to deserialize incoming triggers").Err()
	}
	if len(inv.PropertiesRaw) != 0 {
		props := &structpb.Struct{}
		if err = proto.Unmarshal(inv.PropertiesRaw, props); err != nil {
			return r, errors.Annotate(err, "failed to deserialize properties").Err()
		}
		r.Properties = props
	}
	r.Tags = inv.Tags
	return
}

////////////////////////////////////////////////////////////////////////////////
// requestBuilder methods

func (r *requestBuilder) debugLog(format string, args ...interface{}) {
	debugLog(r.ctx, &r.DebugLog, format, args...)
}

func (r *requestBuilder) prepareNoopRequest(t *scheduler.NoopTrigger) {
	r.Properties = structFromMap(map[string]string{
		"noop_trigger_data": t.Data, // for testing
	})
}

func (r *requestBuilder) prepareGitilesRequest(t *scheduler.GitilesTrigger) {
	repo, err := gitiles.NormalizeRepoURL(t.Repo, false)
	if err != nil {
		r.debugLog("Bad repo URL %q in the trigger - %s", t.Repo, err)
		return
	}
	commit := buildbucket.GitilesCommit{
		Host:     repo.Host,
		Project:  strings.TrimPrefix(repo.Path, "/"),
		Revision: t.Revision,
	}

	r.Properties = structFromMap(map[string]string{
		"revision": t.Revision,
		"branch":   t.Ref,
	})
	r.Tags = []string{
		"buildset:" + commit.String(),
		"gitiles_ref:" + t.Ref,
	}
}

func (r *requestBuilder) prepareBuildbucketRequest(t *scheduler.BuildbucketTrigger) {
	r.Properties = t.Properties
	r.Tags = t.Tags
}
