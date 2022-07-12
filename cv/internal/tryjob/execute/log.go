// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"fmt"

	"go.chromium.org/luci/cv/internal/tryjob"
)

// log adds a new execution log entry.
func (e *Executor) log(entry *tryjob.ExecutionLogEntry) {
	if entry.GetTime() == nil {
		panic(fmt.Errorf("log entry must provide time; got %s", entry))
	}
	// add the entry at the end first and then move it to the right location to
	// ensure logEntries are ordered from oldest to newest.
	e.logEntries = append(e.logEntries, entry)
	for i := len(e.logEntries) - 1; i > 0; i-- {
		if !e.logEntries[i].GetTime().AsTime().Before(e.logEntries[i-1].GetTime().AsTime()) {
			return // found the right spot.
		}
		e.logEntries[i], e.logEntries[i-1] = e.logEntries[i-1], e.logEntries[i]
	}
}
