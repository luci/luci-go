// Copyright 2018 The LUCI Authors.
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

package buildbucket

import "time"

// ParseTimestamp parses a buildbucket timestamp.
func ParseTimestamp(usec int64) time.Time {
	if usec == 0 {
		return time.Time{}
	}
	// TODO(nodir): will start to overflow in year ~2250+, fix it then.
	return time.Unix(0, usec*1000).UTC()
}

// FormatTimestamp converts t to a buildbucket timestamp.
func FormatTimestamp(t time.Time) int64 {
	return t.UnixNano() / 1000
}
