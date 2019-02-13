// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"go.starlark.net/starlark"
)

// sequences is a mapping "name -> int", mutable via `sequence_next()` calls.
type sequences struct {
	s map[string]int
}

// next returns the next number in the given sequence.
//
// Sequences start with 1.
func (s *sequences) next(seq string) int {
	if s.s == nil {
		s.s = make(map[string]int, 1)
	}
	v := s.s[seq] + 1
	s.s[seq] = v
	return v
}

func init() {
	// sequence_next(name) returns a next integer in the sequence.
	declNative("sequence_next", func(call nativeCall) (starlark.Value, error) {
		var name starlark.String
		if err := call.unpack(1, &name); err != nil {
			return nil, err
		}
		return starlark.MakeInt(call.State.seq.next(name.GoString())), nil
	})
}
