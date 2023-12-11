// Copyright 2023 The LUCI Authors.
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

package testvariantbranch

import (
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/span"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// EncodeProtoMessage uses zstd to encode a proto message into []byte.
func EncodeProtoMessage(m proto.Message) ([]byte, error) {
	bytes, err := proto.Marshal(m)
	if err != nil {
		return nil, err
	}
	return span.Compress(bytes), nil
}

func EncodeSegment(seg *cpb.Segment) ([]byte, error) {
	if seg == nil {
		return []byte{}, nil
	}
	return EncodeProtoMessage(seg)
}

func EncodeSegments(segs *cpb.Segments) ([]byte, error) {
	if segs == nil {
		return []byte{}, nil
	}
	return EncodeProtoMessage(segs)
}

func EncodeStatistics(stats *cpb.Statistics) ([]byte, error) {
	if stats == nil {
		return []byte{}, nil
	}
	return EncodeProtoMessage(stats)
}

func EncodeSourceRef(sourceRef *pb.SourceRef) ([]byte, error) {
	if sourceRef == nil {
		panic("source ref should not be nil")
	}
	return EncodeProtoMessage(sourceRef)
}

// DecodeProtoMessage decodes a byte slice into a proto message.
// It is the inverse of EncodeProtoMessage.
func DecodeProtoMessage(bytes []byte, m proto.Message) error {
	buf := make([]byte, len(bytes)*2)
	decompressed, err := span.Decompress(bytes, buf)
	if err != nil {
		return errors.Annotate(err, "decompress").Err()
	}
	return proto.Unmarshal(decompressed, m)
}

// DecodeSegment decodes []byte in to Segment.
func DecodeSegment(bytes []byte) (*cpb.Segment, error) {
	if len(bytes) == 0 {
		return nil, nil
	}
	seg := &cpb.Segment{}
	err := DecodeProtoMessage(bytes, seg)
	if err != nil {
		return nil, err
	}
	return seg, nil
}

// DecodeSegments decodes []byte in to Segments.
func DecodeSegments(bytes []byte) (*cpb.Segments, error) {
	if len(bytes) == 0 {
		return nil, nil
	}
	seg := &cpb.Segments{}
	err := DecodeProtoMessage(bytes, seg)
	if err != nil {
		return nil, err
	}
	return seg, nil
}

// DecodeStatistics decodes []byte into a Statistics proto.
func DecodeStatistics(bytes []byte) (*cpb.Statistics, error) {
	if len(bytes) == 0 {
		return nil, nil
	}
	stats := &cpb.Statistics{}
	err := DecodeProtoMessage(bytes, stats)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// DecodeSourceRef decodes []byte in to SourceRef.
func DecodeSourceRef(bytes []byte) (*pb.SourceRef, error) {
	sourceRef := &pb.SourceRef{}
	err := DecodeProtoMessage(bytes, sourceRef)
	if err != nil {
		return nil, err
	}
	return sourceRef, nil
}
