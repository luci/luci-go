// Copyright 2026 The LUCI Authors.
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

package data

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// Redact removes all data (value.value and value_json), from this Value,
// setting the omitted reason as `NO_ACCESS`.
//
// This retains only the type_urls of the redacted data.
func Redact(val *orchestratorpb.Value) {
	if val == nil {
		return
	}
	val.ClearHasUnknownFields()
	val.ClearValueJson()
	val.SetOmitReason(orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)
	if anyVal := val.GetValue(); anyVal != nil {
		anyVal.Value = nil
	}
}

// Filter will:
//   - Remove data from unwanted types (setting the omit reason to `UNWANTED`).
//   - Replace value data for types where JSONPB encoding is desired (and set
//     has_unknown_fields if this process could not fully reserialize the
//     value).
//
// If set, `mopt.Resolver` will be used for decoding and encoding all data
// where JSONPB encoding is desired. If there is no type available to decode
// in the resolver, this will leave the binary encoded data as-is.
func Filter(val *orchestratorpb.Value, ti *TypeInfo, mopt protojson.MarshalOptions) error {
	if !ti.Wanted.MatchValue(val) {
		Redact(val)
		val.SetOmitReason(orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
		return nil
	}

	if !ti.UnknownJSONPB || ti.Known.MatchValue(val) {
		return nil
	}

	return doJSONPB(val, mopt)
}

// hasUnknownFields recursively walks `msg` and returns true if `msg` or any of
// it's contained message values have unknown fields.
func hasUnknownFields(msg protoreflect.Message) bool {
	hasUnknown := false
	err := protorange.Range(msg, func(v protopath.Values) error {
		if hasUnknown {
			return protorange.Break
		}
		desc := v.Path[len(v.Path)-1]
		if desc.Kind() == protopath.UnknownAccessStep {
			hasUnknown = true
			return protorange.Break
		}
		return nil
	})
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return hasUnknown
}

// doJSONPB sets value_json in this `val` if:
//   - `val` is not nil
//   - `val` does not have the `omitted` field set.
//
// The resolver for decoding the Any and encoding the JSON value will be
// taken from `mopt`, if supplied, or [protoregistry.GlobalTypes], if not.
//
// Returns an error if `val` contains an unknown type, or there was an error
// during marshal/unmarshal.
func doJSONPB(val *orchestratorpb.Value, mopt protojson.MarshalOptions) error {
	if val == nil {
		return nil
	}
	if val.GetOmitReason() != 0 {
		return nil
	}
	resolver := mopt.Resolver
	if resolver == nil {
		resolver = protoregistry.GlobalTypes
	}

	// TODO: There is probably a more efficient conversion mechanism by walking
	// the encoded Any.value data with descriptor in-hand and converting directly
	// to protojson.
	//
	// Such a mechanism would also let us trivially compute HasUnknownFields
	// during this transcode.
	apb := val.GetValue()
	desc, err := resolver.FindMessageByURL(apb.TypeUrl)
	if err != nil {
		return err
	}
	msg := desc.New().Interface()
	if err := (proto.UnmarshalOptions{
		Resolver: resolver,
	}).Unmarshal(apb.Value, msg); err != nil {
		return err
	}

	if hasUnknownFields(msg.ProtoReflect()) {
		val.SetHasUnknownFields(true)
	}

	msgJson, err := mopt.Marshal(msg)
	if err != nil {
		return err
	}
	val.SetValueJson(string(msgJson))

	// TODO: Set Omitted to OMIT_REASON_JSON_ENCODED?
	apb.Value = nil

	return nil
}
