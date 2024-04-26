// Copyright 2024 The LUCI Authors.
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

package bq

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"

	bqpb "go.chromium.org/luci/swarming/proto/bq"
)

// fixers know how to "fix" messages of given proto types.
var fixers map[protoreflect.MessageDescriptor]messageFixer

// messageFixer knows how to "fix" JSON dicts matching particular proto message.
type messageFixer struct {
	// Fixers to apply recursively, keyed by JSON field name.
	perKey map[string]func(in any) (any, error)
}

func init() {
	// Preprocess all exported message types to know what JSON fields need to be
	// post-processed in exportToJSON.
	fixers = map[protoreflect.MessageDescriptor]messageFixer{}
	for _, msg := range []proto.Message{
		&bqpb.TaskRequest{},
		&bqpb.BotEvent{},
		&bqpb.TaskResult{},
	} {
		typ := msg.ProtoReflect().Descriptor()
		if fixer := buildMessageFixer(typ); len(fixer.perKey) != 0 {
			fixers[typ] = fixer
		}
	}
}

// exportToJSON converts a protobuf message to JSON for PubSub export.
//
// It understands `swarming.bq.marshal_to_json_as` field options.
func exportToJSON(msg proto.Message) (string, error) {
	// Only types known in advance are supported here.
	fixer, ok := fixers[msg.ProtoReflect().Descriptor()]
	if !ok {
		panic("unexpected message type")
	}

	// Serialize to JSON using default proto encoding rules.
	raw, err := (protojson.MarshalOptions{
		UseProtoNames: true,
	}).Marshal(msg)
	if err != nil {
		return "", err
	}

	// Convert to a structured Go value that can be "fixed".
	var dict map[string]any
	if err := json.Unmarshal(raw, &dict); err != nil {
		return "", err
	}

	// Recursively "fix" necessary parts.
	if err := fixMsg(dict, fixer); err != nil {
		return "", err
	}

	// Convert back to pretty JSON.
	blob, err := json.MarshalIndent(dict, "", "  ")
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

// fixMsg applies fixes to a JSON dict, recursively.
func fixMsg(msg map[string]any, fixer messageFixer) error {
	for key, val := range msg {
		if val == nil {
			continue // can't fix null
		}
		fixField, ok := fixer.perKey[key]
		if !ok {
			continue // no need to fix this field
		}
		if reflect.TypeOf(val).Kind() == reflect.Slice {
			// This is a slice, apply the fixer to each individual element.
			s := reflect.ValueOf(val)
			for i := 0; i < s.Len(); i++ {
				val := s.Index(i)
				fixed, err := fixField(val.Interface())
				if err != nil {
					return errors.Annotate(err, "%q[%d]", key, i).Err()
				}
				val.Set(reflect.ValueOf(fixed))
			}
		} else {
			// This is some elementary type or a message. Fix it directly.
			fixed, err := fixField(val)
			if err != nil {
				return errors.Annotate(err, "%q", key).Err()
			}
			msg[key] = fixed
		}
	}
	return nil
}

// buildMessageFixer traverses proto message descriptor and builds a fixer.
func buildMessageFixer(typ protoreflect.MessageDescriptor) messageFixer {
	fixer := messageFixer{perKey: map[string]func(in any) (any, error){}}
	fields := typ.Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		jsonKey := string(field.Name())

		// See if this field is annotated with `marshal_to_json_as` option.
		if opts := field.Options(); opts != nil && proto.HasExtension(opts, bqpb.E_MarshalToJsonAs) {
			kind := proto.GetExtension(opts, bqpb.E_MarshalToJsonAs).(bqpb.MarshalKind)
			switch kind {
			case bqpb.MarshalKind_JSON:
				if field.Kind() != protoreflect.StringKind {
					panic("marshal_to_json_as=JSON can be applied only to string fields")
				}
				fixer.perKey[jsonKey] = jsonFixer
			case bqpb.MarshalKind_DURATION:
				if field.Kind() != protoreflect.DoubleKind {
					panic("marshal_to_json_as=DURATION can be applied only to double fields")
				}
				fixer.perKey[jsonKey] = durationFixer
			default:
				panic(fmt.Sprintf("unknown marshal_to_json_as %v", kind))
			}
			continue
		}

		// Maybe some nested message needs fixes.
		if field.Kind() == protoreflect.MessageKind {
			if inner := buildMessageFixer(field.Message()); len(inner.perKey) != 0 {
				fixer.perKey[jsonKey] = func(in any) (any, error) {
					if err := fixMsg(in.(map[string]any), inner); err != nil {
						return nil, err
					}
					return in, nil
				}
			}
		}
	}

	return fixer
}

// jsonFixer knows how to replace a JSON string with a structured JSON dict.
//
// Broken JSON values are silently skipped and replaced with null.
func jsonFixer(in any) (any, error) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(in.(string)), &msg); err != nil {
		return nil, nil
	}
	return msg, nil
}

// durationFixer knows how to replace a double with a proto duration encoding.
func durationFixer(in any) (any, error) {
	secs := in.(float64)
	dur := durationpb.New(time.Duration(secs * float64(time.Second)))
	blob, err := protojson.Marshal(dur)
	if err != nil {
		return nil, err
	}
	// We get JSON encoding, which is a JSON string. Get rid of `"`, they will be
	// added later when we serialize the final result as JSON.
	return strings.Trim(string(blob), `"`), nil
}
