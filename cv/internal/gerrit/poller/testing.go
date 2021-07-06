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

package poller

import (
	"google.golang.org/protobuf/proto"
)

// FilterPayloads returns payloads for Gerrit Poller tasks only.
func FilterPayloads(payloads []proto.Message) []*PollGerritTask {
	var out []*PollGerritTask
	for _, p := range payloads {
		if t, ok := p.(*PollGerritTask); ok {
			out = append(out, t)
		}
	}
	return out
}

// FilterProjects returns Projects from the tasks for Gerrit Poller.
func FilterProjects(payloads []proto.Message) []string {
	out := FilterPayloads(payloads)
	ret := make([]string, len(out))
	for i, p := range out {
		ret[i] = p.GetLuciProject()
	}
	return ret
}
