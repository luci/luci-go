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
// Returns an error if `ref` refers to an unknown type, or there was an error
// during marshal/unmarshal.
//
// Returns an error if `ref` refers to data which is not inlined, and is not
// already in `source`.
func AbsorbAsJSON(source DataSource, ref *orchestratorpb.ValueRef, mopt protojson.MarshalOptions) error {
	AbsorbInline(source, ref)

	dat := source.Retrieve(ref.GetDigest())
	if dat.HasJson() {
		return nil
	}
	if !dat.HasBinary() {
		return fmt.Errorf("could not convert missing data for digest %q", ref.GetDigest())
	}

	jsonPB, err := convertToJson(dat.GetBinary(), mopt)
	if err != nil {
		return err
	}

	source.Intern(map[string]*orchestratorpb.ValueData{
		ref.GetDigest(): orchestratorpb.ValueData_builder{
			Json: jsonPB,
		}.Build(),
	})

	return nil
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

// convertToJson returns a JsonAny given an Any.
func convertToJson(apb *anypb.Any, mopt protojson.MarshalOptions) (*orchestratorpb.ValueData_JsonAny, error) {
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
		return nil, err
	}
	msg := desc.New().Interface()
	if err := (proto.UnmarshalOptions{
		Resolver: resolver,
	}).Unmarshal(apb.Value, msg); err != nil {
		return nil, err
	}

	ret := orchestratorpb.ValueData_JsonAny_builder{
		TypeUrl: &apb.TypeUrl,
	}.Build()

	if hasUnknownFields(msg.ProtoReflect()) {
		ret.SetHasUnknownFields(true)
	}

	msgJson, err := mopt.Marshal(msg)
	if err != nil {
		return nil, err
	}
	ret.SetValue(string(msgJson))

	return ret, nil
}
