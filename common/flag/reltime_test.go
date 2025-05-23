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

package flag

import (
	"testing"
	"time"
)

func TestRelativeTime_Set(t *testing.T) {
	t.Parallel()
	now := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)
	nowFunc := func() time.Time { return now }
	cases := []struct {
		desc  string
		input string
		want  time.Time
	}{
		{
			desc:  "positive",
			input: "5",
			want:  time.Date(2000, 1, 7, 3, 4, 5, 6, time.UTC),
		},
		{
			desc:  "positive with plus",
			input: "+5",
			want:  time.Date(2000, 1, 7, 3, 4, 5, 6, time.UTC),
		},
		{
			desc:  "negative",
			input: "-4",
			want:  time.Date(1999, 12, 29, 3, 4, 5, 6, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			t.Parallel()
			var got time.Time
			f := RelativeTime{T: &got, now: nowFunc}
			if err := f.Set(c.input); err != nil {
				t.Fatalf("Set(%#v) returned error: %s", c.input, err)
			}
			if !got.Equal(c.want) {
				t.Errorf("Set(%#v) = %s; want %s", c.input, got.String(), c.want.String())
			}
		})
	}
}
