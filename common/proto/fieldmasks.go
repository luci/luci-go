// Copyright 2019 The LUCI Authors.
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

package proto

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sync"
	"unicode"

	jsonpbv1 "github.com/golang/protobuf/jsonpb"
	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	fieldMaskType    = reflect.TypeOf((*fieldmaskpb.FieldMask)(nil))
	structType       = reflect.TypeOf((*structpb.Struct)(nil))
	protoMessageType = reflect.TypeOf((*protov1.Message)(nil)).Elem()
)

// UnmarshalJSONWithNonStandardFieldMasks unmarshals a JSONPB message that has
// google.protobuf.FieldMask inside that either uses non-standard field mask
// semantics (like paths that contains `*`) or uses non-standard object encoding
// (e.g. `"mask": {"paths": ["a", "b"]}` instead of `"mask": "a,b"`), or both.
//
// Such field masks are not supported by the standard JSONPB unmarshaller.
// If your message uses only standard field masks (i.e. only containing paths
// like `a.b.c` with not extra syntax, with the field mask serialized as a
// string), use the standard JSON unmarshaler from
// google.golang.org/protobuf/encoding/protojson package.
//
// Using non-standard field masks is discouraged. Deserializing them has
// significant performance overhead. This function is also more likely to break
// in the future (since it uses deprecated libraries under the hood).
//
// Giant name of this function is a hint that it should not be used.
func UnmarshalJSONWithNonStandardFieldMasks(buf []byte, msg proto.Message) error {
	v1 := protov1.MessageV1(msg)
	t := reflect.TypeOf(v1)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	buf, err := fixFieldMasksBeforeUnmarshal(buf, t)
	if err == nil {
		err = (&jsonpbv1.Unmarshaler{AllowUnknownFields: true}).Unmarshal(bytes.NewBuffer(buf), v1)
	}
	return err
}

// fixFieldMasksBeforeUnmarshal reads FieldMask fields from a JSON-encoded
// message, parses them as a string according to
// https://github.com/protocolbuffers/protobuf/blob/ec1a70913e5793a7d0a7b5fbf7e0e4f75409dd41/src/google/protobuf/field_mask.proto#L180
// and converts them to a JSON serialization format that Golang Protobuf v1
// library can unmarshal from.
//
// It is a workaround for https://github.com/golang/protobuf/issues/745.
//
// messageType must be a struct, not a struct pointer.
func fixFieldMasksBeforeUnmarshal(jsonMessage []byte, messageType reflect.Type) ([]byte, error) {
	var msg map[string]any
	if err := json.Unmarshal(jsonMessage, &msg); err != nil {
		return nil, err
	}

	if err := fixFieldMasks(msg, messageType); err != nil {
		return nil, err
	}

	return json.Marshal(msg)
}

func fixFieldMasks(msg map[string]any, messageType reflect.Type) error {
	fieldTypes := getFieldTypes(messageType)
	for name, val := range msg {
		typ := fieldTypes[name]
		if typ == nil {
			continue // no such field, this is fine, since we decode with `AllowUnknownFields: true`
		}

		switch val := val.(type) {
		case string:
			if typ == fieldMaskType {
				msg[name] = convertFieldMask(val)
			}

		case map[string]any:
			if typ == fieldMaskType {
				convertObjectFieldMask(val)
			} else if typ != structType && typ.Implements(protoMessageType) {
				if err := fixFieldMasks(val, typ.Elem()); err != nil {
					return err
				}
			}

		case []any:
			if typ.Kind() == reflect.Slice && typ.Elem().Implements(protoMessageType) {
				subMsgType := typ.Elem().Elem()
				for _, el := range val {
					if subMsg, ok := el.(map[string]any); ok {
						if err := fixFieldMasks(subMsg, subMsgType); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

// convertFieldMask converts a FieldMask from a string according to
// https://github.com/protocolbuffers/protobuf/blob/ec1a70913e5793a7d0a7b5fbf7e0e4f75409dd41/src/google/protobuf/field_mask.proto#L180
// and converts them to a JSON object that Golang Protobuf library understands.
func convertFieldMask(s string) map[string]any {
	paths := parseFieldMaskString(s)
	for i := range paths {
		paths[i] = toSnakeCase(paths[i])
	}
	return map[string]any{
		"paths": paths,
	}
}

// convertObjectFieldMask takes a JSON map with a field mask proto (i.e.
// `{"paths": [...]}`), and converts (in place) paths inside to snake case.
//
// It ignores any extra keys or any type mismatches: all these errors will be
// dealt with when trying to deserialize the resulting JSON for real.
func convertObjectFieldMask(m map[string]any) {
	paths := m["paths"]
	slice, _ := paths.([]any)
	if len(slice) == 0 {
		return
	}
	fixed := make([]string, len(slice))
	for i, p := range slice {
		path, ok := p.(string)
		if !ok {
			return
		}
		fixed[i] = toSnakeCase(path)
	}
	m["paths"] = fixed
}

func toSnakeCase(s string) string {
	buf := &bytes.Buffer{}
	buf.Grow(len(s) + 5) // accounts for 5 underscores
	for _, c := range s {
		if unicode.IsUpper(c) {
			buf.WriteString("_")
			buf.WriteRune(unicode.ToLower(c))
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

// parseFieldMaskString parses a google.protobuf.FieldMask string according to
// https://github.com/protocolbuffers/protobuf/blob/ec1a70913e5793a7d0a7b5fbf7e0e4f75409dd41/src/google/protobuf/field_mask.proto#L180
// Does not convert JSON names (e.g. fooBar) to original names (e.g. foo_bar).
func parseFieldMaskString(s string) (paths []string) {
	inQuote := false
	var seps []int
	for i, c := range s {
		switch {
		case c == '`':
			inQuote = !inQuote

		case inQuote:
			continue

		case c == ',':
			seps = append(seps, i)
		}
	}

	if len(seps) == 0 {
		return []string{s}
	}

	paths = make([]string, 0, len(seps)+1)
	for i := range seps {
		start := 0
		if i > 0 {
			start = seps[i-1] + 1
		}
		paths = append(paths, s[start:seps[i]])
	}
	paths = append(paths, s[seps[len(seps)-1]+1:])
	return paths
}

var fieldTypeCache struct {
	sync.RWMutex
	types map[reflect.Type]map[string]reflect.Type
}

func init() {
	fieldTypeCache.types = map[reflect.Type]map[string]reflect.Type{}
}

// getFieldTypes returns a map from JSON field name to a Go type.
func getFieldTypes(t reflect.Type) map[string]reflect.Type {
	fieldTypeCache.RLock()
	ret, ok := fieldTypeCache.types[t]
	fieldTypeCache.RUnlock()

	if ok {
		return ret
	}

	ret = map[string]reflect.Type{}

	addFieldType := func(p *protov1.Properties, fieldType reflect.Type) {
		ret[p.OrigName] = fieldType
		if p.JSONName != "" {
			// it set only for fields where the JSON name is different.
			ret[p.JSONName] = fieldType
		}
	}

	n := t.NumField()
	fields := make(map[string]reflect.StructField, n)
	for i := range n {
		f := t.Field(i)
		fields[f.Name] = f
	}
	props := protov1.GetProperties(t)
	for _, p := range props.Prop {
		addFieldType(p, fields[p.Name].Type)
	}

	for _, of := range props.OneofTypes {
		addFieldType(of.Prop, of.Type.Elem().Field(0).Type)
	}

	fieldTypeCache.Lock()
	fieldTypeCache.types[t] = ret
	fieldTypeCache.Unlock()
	return ret
}
