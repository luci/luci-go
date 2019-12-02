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
	"math"
	"reflect"
	"testing"
)

func TestEntryJSON(t *testing.T) {
	for _, e := range []entry{
		{
			key:   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
			value: 0,
		},
		{
			key:   "46c5964fc9911cf02d6353d04ddff98aebb56ced",
			value: math.MaxInt32 + 1,
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
			t.Errorf("%v != UnMarshalJSON(%v.MarshalJSON())", unmarshal, e)
		}
	}
}
