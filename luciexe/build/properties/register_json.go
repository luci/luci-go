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

package properties

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

func jsonFromStruct(jsonUseNumber bool, checkUnused bool) func(ctx context.Context, ns string, unknown unknownFieldSetting, s *structpb.Struct, target any) (badExtras bool, err error) {
	return func(ctx context.Context, ns string, unknown unknownFieldSetting, s *structpb.Struct, target any) (badExtras bool, err error) {
		jsonBlob, err := protojson.Marshal(s)
		if err != nil {
			return false, errors.Annotate(err, "jsonFromStruct[%T]", target).Err()
		}
		dec := json.NewDecoder(bytes.NewReader(jsonBlob))
		if jsonUseNumber {
			dec.UseNumber()
		}
		if err := dec.Decode(target); err != nil {
			return false, errors.Annotate(err, "jsonFromStruct[%T]", target).Err()
		}
		if !checkUnused {
			return false, nil
		}

		toSubtract := []*structpb.Struct{{}}
		buf := bytes.NewBuffer(jsonBlob[:0])
		if err := json.NewEncoder(buf).Encode(target); err != nil {
			return false, errors.Annotate(err, "impossible - could not marshal target to JSON").Err()
		}
		if err := protojson.Unmarshal(buf.Bytes(), toSubtract[0]); err != nil {
			return false, errors.Annotate(err, "impossible - could not unmarshal JSON to Struct").Err()
		}

		return handleInputLogging(ctx, ns, jsonBlob, unknown, s, toSubtract)
	}
}

func anyToJSON(source any) []byte {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(source); err != nil {
		// impossible - we know all the types that `source` could be are json
		// serializable.
		panic(err)
	}
	return buf.Bytes()
}

var jsonToVisibleFields = map[reflect.Type]stringset.Set{}
var jsonToVisibleFieldsMu sync.Mutex

func jsonToVisibleFieldsOf(typ reflect.Type) stringset.Set {
	if typ.Kind() == reflect.Map {
		return nil
	}

	if typ.Kind() != reflect.Pointer {
		panic("impossible")
	}
	el := typ.Elem()
	if el.Kind() != reflect.Struct {
		panic("impossible")
	}

	jsonToVisibleFieldsMu.Lock()
	defer jsonToVisibleFieldsMu.Unlock()

	ret, ok := jsonToVisibleFields[typ]
	if ok {
		return ret
	}

	ret = encodingJSON_typeFields(typ.Elem())
	jsonToVisibleFields[typ] = ret

	return ret
}

// encodingJSON_typeFields was copied and heavily modified from
// "encoding/json".typeFields() (in encode.go) in order to accurately replicate
// encoding/json's field visibility rules. Everything not related to the calculation
// of the unordered set of field names has been removed.
//
// Original code is Copyright 2010 The Go Authors and governed by Go's BSD-style
// LICENSE file (https://go.googlesource.com/go/+/refs/tags/go1.23.0/LICENSE).
func encodingJSON_typeFields(t reflect.Type) stringset.Set {
	// Anonymous fields to explore at the current level and the next.
	current := []reflect.Type{}
	next := []reflect.Type{t}

	// Types already visited at an earlier level.
	visited := map[reflect.Type]bool{}

	ret := stringset.New(5)

	for len(next) > 0 {
		current, next = next, current[:0]
		nextCount := map[reflect.Type]int{}

		for _, f := range current {
			if visited[f] {
				continue
			}
			visited[f] = true

			// Scan f for fields to include.
			for i := 0; i < f.NumField(); i++ {
				sf := f.Field(i)
				if sf.Anonymous {
					t := sf.Type
					if t.Kind() == reflect.Pointer {
						t = t.Elem()
					}
					if !sf.IsExported() && t.Kind() != reflect.Struct {
						// Ignore embedded fields of unexported non-struct types.
						continue
					}
					// Do not ignore embedded fields of unexported struct types
					// since they may have exported fields.
				} else if !sf.IsExported() {
					// Ignore unexported non-embedded fields.
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, _, _ := strings.Cut(tag, ",")

				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					// Follow pointer.
					ft = ft.Elem()
				}

				// Record found field.
				if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
					if name == "" {
						name = sf.Name
					}
					ret.Add(name)

					continue
				}

				// Record new anonymous struct to explore in next round.
				nextCount[ft]++
				if nextCount[ft] == 1 {
					next = append(next, ft)
				}
			}
		}
	}

	return ret
}
