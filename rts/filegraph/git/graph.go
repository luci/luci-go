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

package git

import "go.chromium.org/luci/rts/filegraph"

// Graph implements filegraph.Graph.
type Graph struct {
}

// Node returns a node by name.
func (g *Graph) Node(name filegraph.Name) filegraph.Node {
	// TODO(1136280): implement.
	panic("not implemented")
}

func (g *Graph) addCommit(c commit) error {
	// TODO(1136280): implement.
	panic("not implemented")
}
