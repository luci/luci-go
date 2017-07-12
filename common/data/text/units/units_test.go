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

package units

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSizeToString(t *testing.T) {
	t.Parallel()
	Convey(`A size unit should convert to a valid string representation.`, t, func() {
		data := []struct {
			in       int64
			expected string
		}{
			{0, "0b"},
			{1, "1b"},
			{1000, "1000b"},
			{1023, "1023b"},
			{1024, "1.00Kib"},
			{1029, "1.00Kib"},
			{1030, "1.01Kib"},
			{10234, "9.99Kib"},
			{10239, "10.00Kib"},
			{10240, "10.0Kib"},
			{1048575, "1024.0Kib"},
			{1048576, "1.00Mib"},
		}
		for _, line := range data {
			So(SizeToString(line.in), ShouldResemble, line.expected)
		}
	})
}

func TestRound(t *testing.T) {
	t.Parallel()
	Convey(`Unit values should round correctly.`, t, func() {
		data := []struct {
			in       time.Duration
			round    time.Duration
			expected time.Duration
		}{
			{-time.Second, time.Second, -time.Second},
			{-500 * time.Millisecond, time.Second, -time.Second},
			{-499 * time.Millisecond, time.Second, 0},
			{0, time.Second, 0},
			{499 * time.Millisecond, time.Second, 0},
			{500 * time.Millisecond, time.Second, time.Second},
			{time.Second, time.Second, time.Second},
		}
		for _, line := range data {
			So(Round(line.in, line.round), ShouldResemble, line.expected)
		}
	})
}
