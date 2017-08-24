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

// Package jsontime implements a JSON-serializable container for a time.Time.
package jsontime

import (
	"encoding/json"
	"time"
)

// Time is a wrapper around a time.Time that can be marshalled to/from JSON.
//
// Time will always unmarshal to UTC, regardless of the input Time's Location.
type Time struct {
	time.Time
}

var _ interface {
	json.Marshaler
	json.Unmarshaler
} = (*Time)(nil)

// MarshalJSON implements json.Marshaler.
func (jt Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(jt.UTC().Format(time.RFC3339Nano))
}

// MarshalJSON implements json.Unmarshaler.
func (jt *Time) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	var err error
	jt.Time, err = time.Parse(time.RFC3339Nano, s)
	return err
}
