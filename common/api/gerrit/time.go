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

package gerrit

import (
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
)

const (
	// GerritTimestampLayout is the timestamp format used in Gerrit.
	//
	// Timestamp is given in UTC and has the format 'yyyy-mm-dd hh:mm:ss.fffffffff' where
	// 'ffffffffff' represents nanoseconds. See:
	// https://gerrit-review.googlesource.com/Documentation/rest-api.html#timestamp
	GerritTimestampLayout = "2006-01-02 15:04:05.000000000"
)

// FormatTime formats time.Time for Gerrit consumption.
func FormatTime(t time.Time) string {
	return fmt.Sprintf(`"%s"`, t.UTC().Format(GerritTimestampLayout))
}

// ParseTime returns time.Time givne its Gerrit representation.
func ParseTime(s string) (time.Time, error) {
	const msg = "failed to parse Gerrit timestamp %q"
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return time.Time{}, errors.Fmt(msg, s)
	}
	t, err := time.Parse(GerritTimestampLayout, s[1:len(s)-1])
	if err != nil {
		return time.Time{}, errors.Annotate(err, msg, s).Err()
	}
	return t, nil
}

// Timestamp is time.Time for Gerrit JSON interop.
type Timestamp struct {
	time.Time
}

// MarshalJSON implements json.Marshaler.
func (t *Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(FormatTime(t.Time)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	parsedTime, err := ParseTime(string(b))
	if err == nil {
		t.Time = parsedTime
	}
	return err
}
