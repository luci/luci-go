// Copyright 2015 The LUCI Authors.
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

package field

import (
	"fmt"
	"hash/fnv"
)

// Canonicalize returns a copy of fieldVals converted to the canonical types for
// the metric's fields (string, int64, or bool).  Canonicalize returns an error
// if fieldVals is the wrong length or contains value of the wrong type.
func Canonicalize(fields []Field, fieldVals []any) ([]any, error) {
	if len(fieldVals) != len(fields) {
		return nil, fmt.Errorf("metric: got %d field values, want %d: %v",
			len(fieldVals), len(fields), fieldVals)
	}

	out := make([]any, 0, len(fields))
	for i, f := range fields {
		fv, ok := fieldVals[i], false

		switch f.Type {
		case StringType:
			_, ok = fv.(string)
		case BoolType:
			_, ok = fv.(bool)
		case IntType:
			if _, ok = fv.(int64); !ok {
				if fvi, oki := fv.(int); oki {
					fv, ok = int64(fvi), true
				} else if fvi, oki := fv.(int32); oki {
					fv, ok = int64(fvi), true
				}
			}
		}

		if !ok {
			return nil, fmt.Errorf(
				"metric: field %s = %T(%v), want %v", f.Name, fv, fv, f.Type)
		}
		out = append(out, fv)
	}
	return out, nil
}

// Hash returns a uint64 hash of fieldVals.
func Hash(fieldVals []any) uint64 {
	if len(fieldVals) == 0 {
		// Avoid allocating the hasher if there are no fieldVals
		return 0
	}
	h := fnv.New64a()

	for _, v := range fieldVals {
		switch v := v.(type) {
		case string:
			h.Write([]byte(v))
		case int64:
			b := [8]byte{}
			for i := range 8 {
				b[i] = byte(v & 0xFF)
				v >>= 8
			}
			h.Write(b[:])
		case bool:
			if v {
				h.Write([]byte{1})
			} else {
				h.Write([]byte{0})
			}
		}
	}
	return h.Sum64()
}
