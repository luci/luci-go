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

package internal

import (
	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
)

// NoopTrigger constructs a noop trigger proto with given ID and data payload.
//
// No other fields are populated.
func NoopTrigger(id, data string) *Trigger {
	return &Trigger{
		Id:      id,
		Payload: &Trigger_Noop{Noop: &api.NoopTrigger{Data: data}},
	}
}
