// Copyright 2022 The LUCI Authors.
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

package msgpackpb

import (
	"bytes"
	"reflect"
	"sort"

	"github.com/vmihailenco/msgpack/v5"

	"go.chromium.org/luci/common/errors"
)

func sortedKeys(mapValue reflect.Value) (keys []reflect.Value, arrayLike bool, err error) {
	keys = mapValue.MapKeys()

	var sortFn func(i, j int) bool
	checkArray := func() {}
	switch mapValue.Type().Key().Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sortFn = func(i, j int) bool { return keys[i].Uint() < keys[j].Uint() }
		checkArray = func() {
			if keys[0].Uint() == 1 && keys[len(keys)-1].Uint() == uint64(len(keys)) {
				arrayLike = true
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sortFn = func(i, j int) bool { return keys[i].Int() < keys[j].Int() }
		checkArray = func() {
			if keys[0].Int() == 1 && keys[len(keys)-1].Int() == int64(len(keys)) {
				arrayLike = true
			}
		}
	case reflect.String:
		sortFn = func(i, j int) bool { return keys[i].String() < keys[j].String() }
	case reflect.Bool:
		sortFn = func(i, j int) bool {
			a, b := keys[i].Bool(), keys[j].Bool()
			return !a && b
		}
	default:
		err = errors.Reason("cannot sort keys of type %s", keys[0].Type()).Err()
		return
	}

	if len(keys) > 1 {
		sort.Slice(keys, sortFn)
		checkArray()
	}

	return
}

// Unfortunately, the Go msgpack doesn't support deterministic map encoding for
// all map key types :(.
//
// Fortunately, such an encoding for the subset of msgpack we use is relatively
// easy.
func msgpackpbDeterministicEncode(val reflect.Value) (msgpack.RawMessage, error) {
	buf := bytes.Buffer{}
	enc := msgpack.GetEncoder()
	enc.Reset(&buf)
	enc.UseCompactInts(true)
	enc.UseCompactFloats(true)

	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	var process func(val reflect.Value) error

	process = func(val reflect.Value) error {
		if val.Kind() == reflect.Interface && !val.IsNil() {
			val = val.Elem()
		}

		if val.Kind() == reflect.Slice {
			sliceLen := val.Len()
			must(enc.EncodeArrayLen(sliceLen))
			for i := 0; i < sliceLen; i++ {
				if err := process(val.Index(i)); err != nil {
					return err
				}
			}
			return nil
		}

		if val.Kind() == reflect.Map {
			keys, arrayLike, err := sortedKeys(val)
			if err != nil {
				return err
			}
			if arrayLike {
				must(enc.EncodeArrayLen(len(keys)))
			} else {
				must(enc.EncodeMapLen(len(keys)))
			}
			for _, k := range keys {
				if !arrayLike {
					if err := enc.Encode(k.Interface()); err != nil {
						return err
					}
				}
				if err := process(val.MapIndex(k)); err != nil {
					return err
				}
			}
			return nil
		}

		return enc.Encode(val.Interface())
	}

	if err := process(val); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
