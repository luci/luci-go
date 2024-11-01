// Copyright 2024 The LUCI Authors.
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

package prpc

import (
	"bytes"
	"fmt"

	jsonpbv1 "github.com/golang/protobuf/jsonpb"
	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	luciproto "go.chromium.org/luci/common/proto"
)

// protoCodec defines how to encode protobuf messages.
type protoCodec int

const (
	// Wire encoding using protobuf v1.
	codecWireV1 protoCodec = iota
	// JSONPB encoding using protobuf v1 without any hacks.
	codecJSONV1
	// JSONPB encoding using protobuf v1 and a hack to support string field masks.
	codecJSONV1WithHack
	// Text proto encoding using protobuf v1.
	codecTextV1
	// Wire encoding using protobuf v2.
	codecWireV2
	// JSONPB encoding using protobuf v2 using standard string field masks syntax.
	codecJSONV2
	// Text proto encoding using protobuf v2.
	codecTextV2
)

// Encode serializes the message, appends it to `b` an returns the resulting
// slice.
func (codec protoCodec) Encode(b []byte, m proto.Message) ([]byte, error) {
	switch codec {
	case codecWireV1:
		blob, err := protov1.Marshal(protov1.MessageV1(m))
		if err != nil {
			return b, err
		}
		if b != nil {
			return append(b, blob...), nil
		}
		return blob, nil

	case codecJSONV1:
		blob, err := (&jsonpbv1.Marshaler{}).MarshalToString(protov1.MessageV1(m))
		if err != nil {
			return b, err
		}
		if b != nil {
			return append(b, []byte(blob)...), nil
		}
		return []byte(blob), nil

	case codecJSONV1WithHack:
		return nil, fmt.Errorf("codecJSONV1WithHack marshaller is not supported")

	case codecTextV1:
		blob := (&protov1.TextMarshaler{}).Text(protov1.MessageV1(m))
		if b != nil {
			return append(b, []byte(blob)...), nil
		}
		return []byte(blob), nil

	case codecWireV2:
		return (proto.MarshalOptions{}).MarshalAppend(b, m)

	case codecJSONV2:
		return (protojson.MarshalOptions{}).MarshalAppend(b, m)

	case codecTextV2:
		return (prototext.MarshalOptions{}).MarshalAppend(b, m)

	default:
		panic(fmt.Sprintf("unknown codec %d", codec))
	}
}

// Decode deserializes the message in `b`.
//
// Unknown fields in JSONPB and Text encoding are ignored because otherwise all
// pRPC clients become tightly coupled to the server implementation and will
// break with codes.Internal when the server adds a new field to the response.
func (codec protoCodec) Decode(b []byte, m proto.Message) error {
	switch codec {
	case codecWireV1:
		return protov1.Unmarshal(b, protov1.MessageV1(m))

	case codecJSONV1:
		return (&jsonpbv1.Unmarshaler{AllowUnknownFields: true}).Unmarshal(bytes.NewBuffer(b), protov1.MessageV1(m))

	case codecJSONV1WithHack:
		return luciproto.UnmarshalJSONWithNonStandardFieldMasks(b, m)

	case codecTextV1:
		return protov1.UnmarshalText(string(b), protov1.MessageV1(m))

	case codecWireV2:
		return (proto.UnmarshalOptions{}).Unmarshal(b, m)

	case codecJSONV2:
		return (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(b, m)

	case codecTextV2:
		return (prototext.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(b, m)

	default:
		panic(fmt.Sprintf("unknown codec %d", codec))
	}
}

// Format is the pRPC format this codec implements.
func (codec protoCodec) Format() Format {
	switch codec {
	case codecWireV1, codecWireV2:
		return FormatBinary
	case codecJSONV1, codecJSONV1WithHack, codecJSONV2:
		return FormatJSONPB
	case codecTextV1, codecTextV2:
		return FormatText
	default:
		panic(fmt.Sprintf("unknown codec %d", codec))
	}
}
