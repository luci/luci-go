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

package eval

import (
	"time"
)

const day = 24 * time.Hour
const dayLayout = "2006-01-02"

func isUTCDayStart(t time.Time) bool {
	return t.Location() == time.UTC && t.Truncate(day).Equal(t)
}

func assert(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}
