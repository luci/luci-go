// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package endpoints

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// LogStreamDescriptor is an endpoints-exported version of the
// logpb.LogStreamDescriptor protobuf.
type LogStreamDescriptor struct {
	// Prefix is the stream's prefix.
	Prefix string `json:"prefix,omitempty"`

	// Name is the log stream's name.
	Name string `json:"name,omitempty"`

	// StreamType is the stream type.
	//
	// This will be one of:
	//   text
	//   binary
	//   datagram
	StreamType string `json:"stream_type,omitempty"`

	// ContentType is the stream's content type.
	ContentType string `json:"content_type,omitempty"`

	// Timestamp is the log stream's base timestamp, expressed as an RFC3339
	// string.
	Timestamp string `json:"timestamp,omitempty"`

	// Tags is the set of associated log tags.
	Tags []*LogStreamDescriptorTag `json:"tags,omitempty"`

	// BinaryFileExt, if set, the stream will be joined together during archival
	// to recreate the original stream and made available at
	// <prefix>/+/<name>.ext.
	BinaryFileExt string `json:"binary_file_ext,omitempty"`
}

// LogStreamDescriptorTag is a single tag entry in a LogStreamDescriptor.
type LogStreamDescriptorTag struct {
	// Key is the tag's key.
	Key string `json:"key"`
	// Value is the tag's value. If it is empty, then the tag has no value.
	Value string `json:"value,omitempty"`
}

// DescriptorFromProto builds an endpoints LogStreamDescriptor from the source
// protobuf.
func DescriptorFromProto(p *logpb.LogStreamDescriptor) *LogStreamDescriptor {
	d := LogStreamDescriptor{
		Prefix:        p.Prefix,
		Name:          p.Name,
		BinaryFileExt: p.BinaryFileExt,
	}

	if ts := p.Timestamp; ts != nil {
		d.Timestamp = ToRFC3339(ts.Time())
	}

	if tags := p.Tags; len(tags) > 0 {
		d.Tags = make([]*LogStreamDescriptorTag, len(tags))
		for i, t := range tags {
			d.Tags[i] = &LogStreamDescriptorTag{
				Key:   t.Key,
				Value: t.Value,
			}
		}
	}

	return &d
}

// DescriptorFromSerializedProto builds an endpoints LogStreamDescriptor from
// a binary stream that is a serialized logpb.LogStreamDescriptor.
func DescriptorFromSerializedProto(data []byte) (*LogStreamDescriptor, error) {
	desc := logpb.LogStreamDescriptor{}
	if err := proto.Unmarshal(data, &desc); err != nil {
		return nil, err
	}
	return DescriptorFromProto(&desc), nil
}
