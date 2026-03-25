// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// AbsorbAsJSON will [AbsorbInline] `ref` (populating its digest, and ensuring
// this data is in source), then convert the binary data in source to JSONPB
// and store it in `source`.
//
// The resolver for decoding the binary data and encoding the JSON value will
// be taken from `mopt`, if supplied, or [protoregistry.GlobalTypes], if not.
//
// If ref.type_url is not a known type, sets the ValueData.conversion_failure
// field to NO_DESCRIPTOR. If there is some other error in conversion, sets the
// conversion_failure field to ERROR.
//
// No-op if `ref` is digest based and `source` contains no data for this digest.
func AbsorbAsJSON(source DataSource, ref *orchestratorpb.ValueRef, mopt protojson.MarshalOptions) {
	AbsorbInline(source, ref)
	EnsureJSON(source, Digest(ref.GetDigest()), mopt)
}

// hasUnknownFields recursively walks `msg` and returns true if `msg` or any of
// it's contained message values have unknown fields.
func hasUnknownFields(msg protoreflect.Message) bool {
	hasUnknown := false
	err := protorange.Range(msg, func(v protopath.Values) error {
		desc := v.Path[len(v.Path)-1]
		if desc.Kind() == protopath.UnknownAccessStep {
			hasUnknown = true
			return protorange.Terminate
		}
		return nil
	})
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return hasUnknown
}

func ensureJSONImpl(source DataSource, digest Digest, mopt protojson.MarshalOptions, force bool) {
	dat := source.Retrieve(digest)
	if dat.HasJson() || (!force && dat.HasConversionFailure()) {
		// We either already have the JSON, or we already tried and failed.
		// No need to try again.
		return
	}
	apb := dat.GetBinary()
	if apb == nil {
		// No binary data for this digest.
		return
	}

	source.Intern(digest, ConvertToJSON(apb, mopt))
}

// EnsureJSON ensures that the data in `source` for `digest` is encoded as
// JSON.
//
// No-op if:
//   - The data does not exist in source.
//   - The data is already JSON in source.
//   - The data has a conversion failure in source.
func EnsureJSON(source DataSource, digest Digest, mopt protojson.MarshalOptions) {
	ensureJSONImpl(source, digest, mopt, false)
}

// EnsureJSONForced ensures that the data in `source` for `digest` is encoded as
// JSON.
//
// This ignores existing conversion errors in the source and will always try
// again to serialize the data.
//
// No-op if:
//   - The data does not exist in source.
//   - The data is already JSON in source.
func EnsureJSONForced(source DataSource, digest Digest, mopt protojson.MarshalOptions) {
	ensureJSONImpl(source, digest, mopt, true)
}

// ConvertToJSON returns a ValueData given an Any.
//
// If marshaling is possible, the ValueData will contain a JsonAny. If there is
// an error, the ValueData will contain `apb` as Binary plus a
// conversion_failure.
func ConvertToJSON(apb *anypb.Any, mopt protojson.MarshalOptions) *orchestratorpb.ValueData {
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
	desc, err := resolver.FindMessageByURL(apb.TypeUrl)
	if err != nil {
		return orchestratorpb.ValueData_builder{
			Binary:            apb,
			ConversionFailure: orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_NO_DESCRIPTOR.Enum(),
		}.Build()
	}

	msg := desc.New().Interface()
	if err := (proto.UnmarshalOptions{
		Resolver: resolver,
	}).Unmarshal(apb.Value, msg); err != nil {
		return orchestratorpb.ValueData_builder{
			Binary:            apb,
			ConversionFailure: orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR.Enum(),
		}.Build()
	}

	jpb := orchestratorpb.ValueData_JsonAny_builder{
		TypeUrl: &apb.TypeUrl,
	}.Build()

	if hasUnknownFields(msg.ProtoReflect()) {
		jpb.SetHasUnknownFields(true)
	}

	msgJson, err := mopt.Marshal(msg)
	if err != nil {
		return orchestratorpb.ValueData_builder{
			Binary:            apb,
			ConversionFailure: orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR.Enum(),
		}.Build()
	}
	jpb.SetValue(string(msgJson))

	return orchestratorpb.ValueData_builder{Json: jpb}.Build()
}
