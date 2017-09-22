// Copyright 2017 The LUCI Authors.
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

package testutil

import (
	"time"
)

// Timestamp returns a new UTC-based microsecond Unix timestamp for
// the given date and time.
func Timestamp(yyyy, mm, dd, hour, min, sec int) int64 {
	return time.Date(yyyy, time.Month(mm),
		dd, hour, min, sec,
		0, time.UTC).UnixNano() / 1000
}
