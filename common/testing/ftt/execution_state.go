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

package ftt

// executionState is a 3-state value enum for Test.
type executionState byte

const (
	// descending indicates that we are currently progressing from the top of rootCb
	// down towards `t.target`.
	//
	// In this mode, Run and Parallel calls should be no-ops unless they are
	// named with the next token in `t.target`, in which case we directly execute
	// their callback.
	//
	// We exit the descending state when we either:
	//   * find the leaf: we transition to the `leaf` state
	//   * fail to find the leaf: we transition to the `ascending` state
	//
	// Failure to find the target could happen if the user's test generates test
	// names non-deterministically, and is an error.
	descending executionState = iota

	// leaf means that we are currently running the t.target callback.
	//
	// In this mode, Run and Parallel calls create new subtests.
	//
	// We exit the leaf state when we finish the t.target callback,
	// and we transition to `ascending`.
	//
	// Note: root Run and Parallel calls always start in the `leaf` state.
	leaf

	// ascending means that we either finished executing the leaf, or we
	// determined that the target is unfindable.
	//
	// In this mode, Run and Parallel calls are no-ops.
	//
	// This state is terminal.
	ascending
)
