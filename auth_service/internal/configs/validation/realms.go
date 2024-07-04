// Copyright 2024 The LUCI Authors.
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

package validation

import (
	"fmt"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// findCycle finds a path from `start` to itself in the directed graph.
//
// Note: if the graph has other cycles (that don't have `start`), they are
// ignored.
func findCycle(start string, graph map[string][]string) ([]string, error) {
	explored := stringset.Set{} // Roots of totally explored trees.
	visiting := []string{}      // Stack of nodes currently being traversed.

	var doVisit func(node string) (bool, error)
	doVisit = func(node string) (bool, error) {
		if explored.Has(node) {
			// Been there already; no cycles there that have `start` in them.
			return false, nil
		}

		if contains(visiting, node) {
			// Found a cycle that starts and ends with `node`. Return true if it
			// is a `start` cycle; we don't care otherwise.
			return node == start, nil
		}

		visiting = append(visiting, node)
		parentNodes, ok := graph[node]
		if !ok {
			return false, fmt.Errorf("node %q is unrecognized in the graph", node)
		}
		for _, parent := range parentNodes {
			hasCycle, err := doVisit(parent)
			if err != nil {
				return false, err
			}
			if hasCycle {
				// Found a cycle!
				return true, nil
			}
		}

		lastIndex := len(visiting) - 1
		if lastIndex < 0 {
			return false, errors.New("error finding cycles; visiting stack corrupted")
		}
		var lastVisited string
		visiting, lastVisited = visiting[:lastIndex], visiting[lastIndex]
		if lastVisited != node {
			return false, errors.New("error finding cycles; visiting stack order corrupted")
		}

		// Record the root of the cycle-free subgraph.
		explored.Add(node)
		return false, nil
	}

	hasCycle, err := doVisit(start)
	if err != nil {
		return nil, err
	}
	if !hasCycle {
		return []string{}, nil
	}

	// Close the loop.
	visiting = append(visiting, start)
	return visiting, nil
}
