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

package policy

import (
	"fmt"
	"strings"

	structpb "github.com/golang/protobuf/ptypes/struct"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/strpair"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// RequestBuilder is a task.Request in a process of being prepared.
type RequestBuilder struct {
	task.Request

	env Environment // for logging
}

// DebugLog adds a line to the request log and the triage log.
func (r *RequestBuilder) DebugLog(format string, args ...interface{}) {
	r.Request.DebugLog += fmt.Sprintf(format+"\n", args...)
	if r.env != nil {
		r.env.DebugLog(format, args...)
	}
}

// FromTrigger derives the request properties from the given trigger.
func (r *RequestBuilder) FromTrigger(t *internal.Trigger) {
	switch p := t.Payload.(type) {
	case *internal.Trigger_Cron:
		r.FromCronTrigger(p.Cron)
	case *internal.Trigger_Webui:
		r.FromWebUITrigger(p.Webui)
	case *internal.Trigger_Noop:
		r.FromNoopTrigger(p.Noop)
	case *internal.Trigger_Gitiles:
		r.FromGitilesTrigger(p.Gitiles)
	case *internal.Trigger_Buildbucket:
		r.FromBuildbucketTrigger(p.Buildbucket)
	default:
		r.DebugLog("Unrecognized trigger payload of type %T, ignoring", p)
	}
}

// FromCronTrigger derives the request properties from the given cron trigger.
func (r *RequestBuilder) FromCronTrigger(t *scheduler.CronTrigger) {
	// nothing here for now
}

// FromWebUITrigger derives the request properties from the given web UI
// trigger.
func (r *RequestBuilder) FromWebUITrigger(t *scheduler.WebUITrigger) {
	// nothing here for now
}

// FromNoopTrigger derives the request properties from the given noop trigger.
func (r *RequestBuilder) FromNoopTrigger(t *scheduler.NoopTrigger) {
	r.Properties = structFromMap(map[string]string{
		"noop_trigger_data": t.Data, // for testing
	})
}

// FromGitilesTrigger derives the request properties from the given gitiles
// trigger.
func (r *RequestBuilder) FromGitilesTrigger(t *scheduler.GitilesTrigger) {
	repo, err := gitiles.NormalizeRepoURL(t.Repo, false)
	if err != nil {
		r.DebugLog("Bad repo URL %q in the trigger - %s", t.Repo, err)
		return
	}
	commit := &buildbucketpb.GitilesCommit{
		Host:    repo.Host,
		Project: strings.TrimPrefix(repo.Path, "/"),
		Id:      t.Revision,
	}

	r.Properties = structFromMap(map[string]string{
		"revision":   t.Revision,
		"branch":     t.Ref,
		"repository": t.Repo,
	})
	r.Tags = []string{
		strpair.Format(bbv1.TagBuildSet, protoutil.GitilesBuildSet(commit)),
		// TODO(nodir): remove after switching to ScheduleBuild RPC v2.
		strpair.Format(bbv1.TagBuildSet, "commit/git/"+commit.Id),
		strpair.Format("gitiles_ref", t.Ref),
	}
}

// FromBuildbucketTrigger derives the request properties from the given
// buildbucket trigger.
func (r *RequestBuilder) FromBuildbucketTrigger(t *scheduler.BuildbucketTrigger) {
	r.Properties = t.Properties
	r.Tags = t.Tags
}

////////////////////////////////////////////////////////////////////////////////

// structFromMap constructs protobuf.Struct with string keys and values.
func structFromMap(m map[string]string) *structpb.Struct {
	out := &structpb.Struct{
		Fields: make(map[string]*structpb.Value, len(m)),
	}
	for k, v := range m {
		out.Fields[k] = &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: v},
		}
	}
	return out
}
