// Copyright 2020 The LUCI Authors.
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

// Package pmtest implements tests for working with Project Manager.
package pmtest

import (
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// Projects returns list of projects from tasks for PM.
func Projects(in tqtesting.TaskList) (projects []string) {
	for _, t := range in.SortByETA() {
		switch v := t.Payload.(type) {
		case *internal.PokePMTask:
			projects = append(projects, v.GetLuciProject())
		case *internal.KickPokePMTask:
			projects = append(projects, v.GetLuciProject())
		}
	}
	return projects
}
