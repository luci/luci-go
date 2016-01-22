// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamproto

import (
	"encoding/json"
	"flag"

	"github.com/luci/luci-go/common/flag/flagenum"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// StreamType is a flag- and JSON-compatible wrapper around the StreamType
// protobuf field.
type StreamType logpb.LogStreamDescriptor_StreamType

// DefaultContentType returns the default ContentType for a given stream type.
func (t StreamType) DefaultContentType() types.ContentType {
	switch logpb.LogStreamDescriptor_StreamType(t) {
	case logpb.LogStreamDescriptor_TEXT:
		return types.ContentTypeText

	case logpb.LogStreamDescriptor_DATAGRAM:
		return types.ContentTypeLogdogDatagram

	case logpb.LogStreamDescriptor_BINARY:
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
		"text":     StreamType(logpb.LogStreamDescriptor_TEXT),
		"binary":   StreamType(logpb.LogStreamDescriptor_BINARY),
		"datagram": StreamType(logpb.LogStreamDescriptor_DATAGRAM),
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
