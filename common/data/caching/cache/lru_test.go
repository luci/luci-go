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

package cache

import (
	"crypto"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestEntryJSON(t *testing.T) {
	for _, e := range []entry{
		{
			key:        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
			value:      0,
			lastAccess: time.Now().Unix(),
		},
		{
			key:        "46c5964fc9911cf02d6353d04ddff98aebb56ced",
			value:      math.MaxInt32 + 1,
			lastAccess: time.Now().Unix(),
		},
	} {
		got, err := e.MarshalJSON()
		if err != nil {
			t.Errorf("entry.MarshalJSON() = _, %v, want nil", err)
		}

		var unmarshal entry
		if err := unmarshal.UnmarshalJSON(got); err != nil {
			t.Errorf("entry.UnmarshalJSON() = %v; want nil", err)
		}

		if !reflect.DeepEqual(unmarshal, e) {
			t.Errorf("%#v != UnMarshalJSON(%#v.MarshalJSON())", unmarshal, e)
		}
	}
}

func TestEntryUnmarshal(t *testing.T) {
	for _, data := range []string{
		`["key", [1, 1]]`,
		`["key", [1.0, 1.0]]`,
	} {
		var e entry
		if err := e.UnmarshalJSON([]byte(data)); err != nil {
			t.Fatalf("failed to unmarshal `%s`: %v", data, err)
		}

		if e.key != "key" {
			t.Errorf("got %s for key, expected `key`", e.key)
		}
		if e.value != 1 {
			t.Errorf("got %d for value, expected 1.0", e.value)
		}
		if e.lastAccess != 1 {
			t.Errorf("got %d for lastAccess, expected 1.0", e.lastAccess)
		}
	}
}

func TestLRU(t *testing.T) {
	t.Parallel()

	h := crypto.SHA1
	l := makeLRUDict(h)

	empty := HashBytes(h, nil)
	if got, want := l.touch(empty), false; got != want {
		t.Errorf("l.touch(...)=%v; want %v", got, want)
	}

	l.pushFront(empty, 0)

	if got, want := l.touch(empty), true; got != want {
		t.Errorf("l.touch(...)=%v; want %v", got, want)
	}
}
