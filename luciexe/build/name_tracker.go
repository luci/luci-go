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

package build

import (
	"fmt"
)

type nameTracker struct {
	pool map[string]int
}

func (n *nameTracker) resolveName(requested string) string {
	if n.pool == nil {
		n.pool = map[string]int{}
	}

	// First, do a lookup directly on `requested`. If it's unique, increment it
	// and declare victory.
	dupCount := n.pool[requested]
	if dupCount == 0 {
		n.pool[requested]++
		return requested
	}

	// Now, increment dupCount until we get an empty slot.
	for {
		candidate := fmt.Sprintf("%s (%d)", requested, dupCount+1)
		if n.pool[candidate] == 0 {
			n.pool[candidate]++
			n.pool[requested] = dupCount + 1
			return candidate
		}
		dupCount++
	}
}
