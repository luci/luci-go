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

package streamproto

import (
	"encoding/json"
	"flag"

	"go.chromium.org/luci/logdog/common/types"
)

// StreamNameFlag is a flag and JSON-compatible type that converts to/from a
// types.StreamName. StreamName validation is part of the conversion.
type StreamNameFlag types.StreamName

var _ interface {
	flag.Value
	json.Marshaler
	json.Unmarshaler
} = (*StreamNameFlag)(nil)

// Set implements flag.Value.
func (f *StreamNameFlag) Set(v string) error {
	if err := types.StreamName(v).Validate(); err != nil {
		return err
	}
	*f = StreamNameFlag(v)
	return nil
}

// String implements flag.Value.
func (f *StreamNameFlag) String() string {
	if f == nil {
		return ""
	}
	return string(*f)
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *StreamNameFlag) UnmarshalJSON(data []byte) error {
	s := ""
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if err := types.StreamName(s).Validate(); err != nil {
		return err
	}
	*f = StreamNameFlag(s)
	return nil
}

// MarshalJSON implements json.Marshaler.
func (f StreamNameFlag) MarshalJSON() ([]byte, error) {
	s := string(f)
	return json.Marshal(&s)
}
