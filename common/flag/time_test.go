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

package flag_test

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/flag"
)

func TestTimeFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Tag   string
		Input string
		Want  time.Time
	}{
		{
			Tag:   "riff on default format time",
			Input: "2006-01-02T15:04:05.999999999Z",
			Want:  time.Date(2006, time.January, 2, 15, 4, 5, 999999999, time.UTC),
		},
		{
			Tag:   "typical example from stiptime",
			Input: "2015-06-30T18:50:50.0Z",
			Want:  time.Date(2015, time.June, 30, 18, 50, 50, 0, time.UTC),
		},
	}

	for _, c := range cases {
		t.Run(c.Tag, func(t *testing.T) {
			t.Parallel()
			var got time.Time
			f := flag.Time(&got)
			if err := f.Set(c.Input); err != nil {
				t.Fatalf("Error parsing %s: %s", c.Input, err)
			}
			if c.Want != got {
				t.Errorf("Incorrectly parsed %s, want %s got %s", c.Input, c.Want, got)
			}
		})
	}
}

func TestTimeFlagErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Tag   string
		Input string
	}{
		{
			Tag:   "empty input",
			Input: "",
		},
		{
			Tag:   "leap second example from stiptime",
			Input: "2015-06-30T23:59:60.123Z",
		},
		{
			Tag:   "incorrect timezone suffix",
			Input: "2006-01-02T15:04:05.999999999+07:00",
		},
		{
			Tag:   "incorrect timezone",
			Input: "2006-01-02T15:04:05.999999999-01:00",
		},
	}

	for _, c := range cases {
		t.Run(c.Tag, func(t *testing.T) {
			t.Parallel()
			var got time.Time
			f := flag.Time(&got)
			if err := f.Set(c.Input); err == nil {
				t.Errorf("Successfully parsed corrupted input %s as %s", c.Input, got)
			}
		})
	}
}
