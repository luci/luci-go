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

package prjmanager

import (
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// PokePMTaskRef is used by PM implementationt to add its handler.
var PokePMTaskRef tq.TaskClassRef

func init() {
	PokePMTaskRef = tq.RegisterTaskClass(tq.TaskClass{
		ID:        "poke-pm-task",
		Prototype: &internal.PokePMTask{},
		Queue:     "manage-project",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "async-poke-pm-task",
		Prototype: &internal.AsyncPokePMTask{},
		Queue:     "manage-project",
	})
}
