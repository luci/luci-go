// Copyright 2015 The LUCI Authors.
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

package errors

// Walk performs a depth-first traversal of the supplied error, unfolding it and
// invoke the supplied callback for each layered error recursively. If the
// callback returns true, Walk will continue its traversal.
//
//	- If walk encounters a MultiError, the callback is called once for the
//	  outer MultiError, then once for each inner error.
//	- If walk encounters a Wrapped error, the callback is called for the outer
//	  and inner error.
//	- If an inner error is, itself, a container, Walk will recurse into it.
//
// If err is nil, the callback will not be invoked.
func Walk(err error, fn func(error) bool) {
	_ = walkVisit(err, fn, false)
}

// WalkLeaves is like Walk, but only calls fn on leaf nodes.
func WalkLeaves(err error, fn func(error) bool) {
	_ = walkVisit(err, fn, true)
}

func walkVisit(err error, fn func(error) bool, leavesOnly bool) bool {
	if err == nil {
		return true
	}

	// Call fn if we are not in leavesOnly mode.
	if !(leavesOnly || fn(err)) {
		return false
	}

	switch t := err.(type) {
	case MultiError:
		for _, e := range t {
			if !walkVisit(e, fn, leavesOnly) {
				return false
			}
		}

	case Wrapped:
		return walkVisit(t.Unwrap(), fn, leavesOnly)

	default:
		if leavesOnly {
			return fn(err)
		}
	}

	return true
}

// Any performs a Walk traversal of an error, returning true (and
// short-circuiting) if the supplied filter function returns true for any
// visited error.
//
// If err is nil, Any will return false.
func Any(err error, fn func(error) bool) (any bool) {
	Walk(err, func(err error) bool {
		any = fn(err)
		return !any
	})
	return
}

// Contains performs a Walk traversal of |outer|, returning true if any visited
// error is equal to |inner|.
func Contains(outer error, inner error) bool {
	return Any(outer, func(item error) bool {
		if item == inner {
			return true
		}
		if is, ok := item.(interface{ Is(error) bool }); ok {
			return is.Is(inner)
		}
		return false
	})
}
