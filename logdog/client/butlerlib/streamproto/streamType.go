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

	"go.chromium.org/luci/common/flag/flagenum"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// StreamType is a flag- and JSON-compatible wrapper around the StreamType
// protobuf field.
type StreamType logpb.StreamType

// DefaultContentType returns the default ContentType for a given stream type.
func (t StreamType) DefaultContentType() types.ContentType {
	switch logpb.StreamType(t) {
	case logpb.StreamType_TEXT:
		return types.ContentTypeText

	case logpb.StreamType_DATAGRAM:
		return types.ContentTypeLogdogDatagram

	case logpb.StreamType_BINARY:
		fallthrough
	default:
		return types.ContentTypeBinary
	}
}

var _ interface {
	json.Marshaler
	json.Unmarshaler
	flag.Value
} = (*StreamType)(nil)

var (
	// StreamTypeFlagEnum maps configuration strings to their underlying StreamTypes.
	StreamTypeFlagEnum = flagenum.Enum{
		"text":     StreamType(logpb.StreamType_TEXT),
		"binary":   StreamType(logpb.StreamType_BINARY),
		"datagram": StreamType(logpb.StreamType_DATAGRAM),
	}
)

// Set implements flag.Value.
func (t *StreamType) Set(v string) error {
	return StreamTypeFlagEnum.FlagSet(t, v)
}

// String implements flag.Value.
func (t *StreamType) String() string {
	return StreamTypeFlagEnum.FlagString(t)
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *StreamType) UnmarshalJSON(data []byte) error {
	return StreamTypeFlagEnum.JSONUnmarshal(t, data)
}

// MarshalJSON implements json.Marshaler.
func (t StreamType) MarshalJSON() ([]byte, error) {
	return StreamTypeFlagEnum.JSONMarshal(t)
}
