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

package stringset

type set map[string]struct{}

// New returns a new NON-THREAD-SAFE string Set implementation.
func New(sizeHint int) Set {
	return make(set, sizeHint)
}

// NewFromSlice returns a new NON-THREAD-SAFE string Set implementation,
// initialized with the values in the provided slice.
func NewFromSlice(vals ...string) Set {
	ret := New(len(vals))
	for _, v := range vals {
		ret.Add(v)
	}
	return ret
}

func (s set) Has(value string) bool {
	_, ret := s[value]
	return ret
}

func (s set) Add(value string) bool {
	ret := !s.Has(value)
	s[value] = struct{}{}
	return ret
}

func (s set) Del(value string) bool {
	ret := s.Has(value)
	delete(s, value)
	return ret
}

func (s set) Peek() (string, bool) {
	for k := range s {
		return k, true
	}
	return "", false
}

func (s set) Pop() (string, bool) {
	for k := range s {
		s.Del(k)
		return k, true
	}
	return "", false
}

func (s set) Iter(cb func(string) bool) {
	for v := range s {
		if !cb(v) {
			break
		}
	}
}

func (s set) Len() int {
	return len(s)
}

func (s set) Dup() Set {
	ret := New(len(s))
	for v := range s {
		ret.Add(v)
	}
	return ret
}

func (s set) ToSlice() []string {
	ret := make([]string, 0, len(s))
	s.Iter(func(val string) bool {
		ret = append(ret, val)
		return true
	})
	return ret
}

func (s set) Intersect(other Set) Set {
	o := other.(set)
	smallLen := len(s)
	if lo := len(o); lo < smallLen {
		smallLen = lo
	}
	if smallLen == 0 {
		return New(0)
	}
	ret := New(smallLen)
	for k := range s {
		if _, ok := o[k]; ok {
			ret.Add(k)
		}
	}
	return ret
}

func (s set) Difference(other Set) Set {
	o := other.(set)
	ret := New(0)
	for k := range s {
		if _, ok := o[k]; !ok {
			ret.Add(k)
		}
	}
	return ret
}

func (s set) Union(other Set) Set {
	ret := s.Dup()
	for k := range other.(set) {
		ret.Add(k)
	}
	return ret
}
