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

package internal

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
)

// ToPublicTrigger converts Trigger to public *scheduler.Trigger.
func ToPublicTrigger(t *Trigger) *scheduler.Trigger {
	ret := &scheduler.Trigger{
		Id:    t.Id,
		Title: t.Title,
		Url:   t.Url,
	}

	switch p := t.Payload.(type) {
	case *Trigger_Cron:
		ret.Payload = &scheduler.Trigger_Cron{Cron: p.Cron}
	case *Trigger_Buildbucket:
		ret.Payload = &scheduler.Trigger_Buildbucket{Buildbucket: p.Buildbucket}
	case *Trigger_Gitiles:
		ret.Payload = &scheduler.Trigger_Gitiles{Gitiles: p.Gitiles}
	case *Trigger_Noop:
		ret.Payload = &scheduler.Trigger_Noop{}
	case *Trigger_Webui:
		ret.Payload = &scheduler.Trigger_Webui{Webui: p.Webui}
	default:
		panic(fmt.Sprintf("unexpected payload type %T: %s", p, p))
	}

	return ret
}

// isTrivialBuildbucketTrigger is true if the trigger has no properties and
// only sets well-formed "key:value" tags with keys for which
// protoutil.IsBenignTagKey is true, i.e. tags Buildbucket allows to be set
// without buildbucket.builds.addWithParams.
//
// Note that the Buildbucket task manager always adds its own user_agent tag,
// so a trigger with a user_agent tag results in a build with two user_agent
// tags. This is intended.
func isTrivialBuildbucketTrigger(t *scheduler.BuildbucketTrigger) bool {
	if t == nil {
		// Preserve historical behavior: proto.Equal(nil, &{}) is false.
		return false
	}
	for _, tag := range t.Tags {
		key, _, ok := strings.Cut(tag, ":")
		if !ok || !protoutil.IsBenignTagKey(key) {
			return false
		}
	}
	// Check the remaining fields, so that any new field is considered tweakable
	// by default.
	stripped := proto.CloneOf(t)
	stripped.Tags = nil
	return proto.Equal(stripped, &scheduler.BuildbucketTrigger{})
}

// IsTrivialTrigger is true if the trigger's payload doesn't have any tweakable
// parts that can affect what ends up running when the trigger is consumed.
//
// Buildbucket build tags with keys for which protoutil.IsBenignTagKey is
// true are not considered tweakable.
func IsTrivialTrigger(t *Trigger) bool {
	switch p := t.GetPayload().(type) {
	case nil:
		return true
	case *Trigger_Cron:
		return false
	case *Trigger_Buildbucket:
		return isTrivialBuildbucketTrigger(p.Buildbucket)
	case *Trigger_Gitiles:
		return false
	case *Trigger_Noop:
		return true
	case *Trigger_Webui:
		return true
	default:
		panic(fmt.Sprintf("unexpected payload type %T: %s", p, p))
	}
}
