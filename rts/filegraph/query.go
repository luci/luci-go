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

package filegraph

import (
	"go.chromium.org/luci/common/errors"
)

// Query retrieves nodes in the order from closest to furthest,
// relative to roots.
type Query struct {
	Roots []Node
	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add MaxDistance
	// TODO(crbug.com/1136280): add SilbingDistance
}

// Result is one entry in the query result set.
type Result struct {
	Prev     *Result
	Node     Node
	Distance float64
}

// ErrStop when returned by a callback indicates that an iteration must stop.
var ErrStop = errors.New("stop the iteration")

// Run calls the callback for each node reachable from any of the roots.
// The nodes are reported in the ascending distance order relative to the roots,
// starting from the roots themselves.
//
// If the callback returns ErrStop, then the iteration stops.
func (q *Query) Run(g Graph, callback func(result *Result) error) error {
	// TODO(1136280): implement.
	panic("not implemented")
}
