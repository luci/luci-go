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

package base

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	marshalOpts = protojson.MarshalOptions{UseProtoNames: true}
	protoType   = reflect.TypeOf((*proto.Message)(nil)).Elem()
)

type legacyJSON struct{ data any }

type listWithProjection struct {
	items      any                  // []T preserving the original type
	count      int                  // how many items there is
	stdoutLine func(idx int) string // a line matching the corresponding item
}

// LegacyJSON indicates to EncodeJSON to use legacy JSON encoding.
func LegacyJSON(data any) legacyJSON { return legacyJSON{data} }

// ListWithStdoutProjection, when returned by a subcommands implementation,
// means the subcommand should write the JSON list to `-json-output`, but also
// print a line to stdout for every item in the list (the line is produced by
// the given callback).
//
// If `-json-output` is stdout itself, `stdoutLine` takes precedence, i.e.
// stdout will receive a list of lines, not a JSON list.
//
// EncodeJSON treats ListWithStdoutProjection just like a slice of []T.
func ListWithStdoutProjection[T any](items []T, stdoutLine func(t T) string) *listWithProjection {
	return &listWithProjection{
		items:      items,
		count:      len(items),
		stdoutLine: func(idx int) string { return stdoutLine(items[idx]) },
	}
}

// EncodeJSON encodes subcommand results as JSON.
//
// If `m` is a proto.Message, uses protojson encoder for it.
//
// If `m` is a slice, uses protojson encoder for individual entries and
// assembles them in a JSON list. Panics if they are not proto.Message.
//
// If `m` is a map, uses protojson encoder for individual entries and assembles
// them in a JSON dict. Panics if they are not proto.Message or the map uses
// non-string keys.
//
// If `m` is LegacyJSON(...) marshals it using stdlib `encoding/json`.
//
// If `m` is ListWithStdoutProjection(...) treats it as a slice.
//
// TODO(vadimsh): Remove LegacyJSON once crbug.com/894045 is fixed and
// -task-summary-python is removed.
func EncodeJSON(m any) ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}

	// Unwrap listWithProjection, we don't care about the projection part here.
	if wrapper, ok := m.(*listWithProjection); ok {
		m = wrapper.items
	}

	if pb, _ := m.(proto.Message); pb != nil {
		return indent(marshalOpts.Marshal(pb))
	}
	if legacy, ok := m.(legacyJSON); ok {
		return json.MarshalIndent(legacy.data, "", " ")
	}

	val := reflect.ValueOf(m)

	if val.Kind() == reflect.Slice {
		if !val.Type().Elem().AssignableTo(protoType) {
			panic(fmt.Sprintf("%s is not proto.Message", val.Type().Elem()))
		}
		var buf bytes.Buffer
		buf.WriteString("[")
		for i := 0; i < val.Len(); i++ {
			if i != 0 {
				buf.WriteString(",")
			}
			blob, err := marshalOpts.Marshal(val.Index(i).Interface().(proto.Message))
			if err != nil {
				return nil, err
			}
			buf.Write(blob)
		}
		buf.WriteString("]")
		return indent(buf.Bytes(), nil)
	}

	if val.Kind() == reflect.Map {
		if val.Type().Key().Kind() != reflect.String {
			panic(fmt.Sprintf("%s is not string", val.Type().Key()))
		}
		if !val.Type().Elem().AssignableTo(protoType) {
			panic(fmt.Sprintf("%s is not proto.Message", val.Type().Elem()))
		}

		type pair struct {
			key string
			val proto.Message
		}
		pairs := make([]pair, 0, val.Len())

		iter := val.MapRange()
		for iter.Next() {
			pairs = append(pairs, pair{
				key: iter.Key().Interface().(string),
				val: iter.Value().Interface().(proto.Message),
			})
		}

		sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })

		var buf bytes.Buffer
		buf.WriteString("{")
		for i, pair := range pairs {
			if i != 0 {
				buf.WriteString(",")
			}
			keyBlob, err := json.Marshal(pair.key)
			if err != nil {
				return nil, err
			}
			valBlob, err := marshalOpts.Marshal(pair.val)
			if err != nil {
				return nil, err
			}
			buf.Write(keyBlob)
			buf.WriteString(":")
			buf.Write(valBlob)
		}
		buf.WriteString("}")
		return indent(buf.Bytes(), nil)
	}

	panic(fmt.Sprintf("unsupported type %T", val))
}

// indent normalizes spaces in the protojson encoder output.
//
// See https://github.com/golang/protobuf/issues/1082.
func indent(blob []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err = json.Indent(&buf, blob, "", " "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
