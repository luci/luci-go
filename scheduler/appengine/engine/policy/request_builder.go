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

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
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
func (r *RequestBuilder) DebugLog(format string, args ...any) {
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
	r.Properties = mergeIntoStruct(&structpb.Struct{}, map[string]string{
		"noop_trigger_data": t.Data, // for testing
	})
}

// FromGitilesTrigger derives the request properties from the given gitiles
// trigger.
//
// TODO(crbug.com/1182002): Remove properties/tags modifications here once there
// are no in-flight triggers that might hit Buildbucket v1 code path.
func (r *RequestBuilder) FromGitilesTrigger(t *scheduler.GitilesTrigger) {
	repo, err := gitiles.NormalizeRepoURL(t.Repo, false)
	if err != nil {
		r.DebugLog("Bad repo URL %q in the trigger - %s", t.Repo, err)
		return
	}

	// Merge properties derived from the commit info on top t.Properties.
	if t.Properties != nil && len(t.Properties.Fields) != 0 {
		r.Properties = proto.Clone(t.Properties).(*structpb.Struct)
	} else {
		r.Properties = &structpb.Struct{
			Fields: make(map[string]*structpb.Value, 3),
		}
	}
	mergeIntoStruct(r.Properties, map[string]string{
		"revision":   t.Revision,
		"branch":     t.Ref,
		"repository": t.Repo,
	})

	commit := &buildbucketpb.GitilesCommit{
		Host:    repo.Host,
		Project: strings.TrimPrefix(repo.Path, "/"),
		Id:      t.Revision,
	}

	// Join t.Tags with tags derived from the commit.
	r.Tags = make([]string, 0, len(t.Tags)+3)
	r.Tags = append(
		r.Tags,
		strpair.Format("buildset", protoutil.GitilesBuildSet(commit)),
		strpair.Format("gitiles_ref", t.Ref),
	)
	r.Tags = append(r.Tags, t.Tags...)
	r.Tags = removeDups(r.Tags)
}

// FromBuildbucketTrigger derives the request properties from the given
// buildbucket trigger.
func (r *RequestBuilder) FromBuildbucketTrigger(t *scheduler.BuildbucketTrigger) {
	r.Properties = t.Properties
	r.Tags = t.Tags
}

////////////////////////////////////////////////////////////////////////////////

// mergeIntoStruct merges `m` into protobuf.Struct returning it.
func mergeIntoStruct(s *structpb.Struct, m map[string]string) *structpb.Struct {
	if s.Fields == nil {
		s.Fields = make(map[string]*structpb.Value, len(m))
	}
	for k, v := range m {
		s.Fields[k] = &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: v},
		}
	}
	return s
}

// removeDups removes duplicates from the list, modifying it in-place.
func removeDups(l []string) []string {
	seen := stringset.New(len(l))
	filtered := l[:0]
	for _, s := range l {
		if seen.Add(s) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
