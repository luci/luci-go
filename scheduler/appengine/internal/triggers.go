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
	fmt "fmt"

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
