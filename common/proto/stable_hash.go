// Copyright 2023 The LUCI Authors.
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

package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

const anyName = "google.protobuf.Any"

// StableHash does a deterministic and ordered walk over the proto message `m`,
// feeding each field into `h`.
//
// This is useful to produce stable, deterministic hashes of proto messages
// where comparison of messages generated from different sources or runtimes is
// important.
//
// Because of this, `m` may not contain any unknown fields, since there is no
// way to canonicalize them without the message definition. If the message has
// any unknown fields, this function returns an error.
// NOTE:
// - The hash value can ONLY be used for backward compatible protobuf change to
// determine if the protobuf message is different. If two protobuf specs are
// incompatible, their value MUST NOT be compared with each other and not
// promised to be different even their messages are clearly different.
// - google.protobuf.Any is supported via Any.UnmarshalNew: if the message is
// not registered, this returns an error.
// - 0-valued scalar fields are not distinguished from absent fields.
func StableHash(h hash.Hash, m proto.Message) error {
	return hashMessage(h, m.ProtoReflect())
}

func hashValue(h hash.Hash, v protoreflect.Value) error {
	switch v := v.Interface().(type) {
	case int32:
		return hashNumber(h, uint64(v))
	case int64:
		return hashNumber(h, uint64(v))
	case uint32:
		return hashNumber(h, uint64(v))
	case uint64:
		return hashNumber(h, v)
	case float32:
		return hashNumber(h, uint64(math.Float32bits(v)))
	case float64:
		return hashNumber(h, math.Float64bits(v))
	case string:
		return hashBytes(h, []byte(v))
	case []byte:
		return hashBytes(h, v)
	case protoreflect.EnumNumber:
		return hashNumber(h, uint64(v))
	case protoreflect.Message:
		return hashMessage(h, v)
	case protoreflect.List:
		return hashList(h, v)
	case protoreflect.Map:
		return hashMap(h, v)
	case bool:
		var b uint64
		if v {
			b = 1
		}
		return hashNumber(h, b)
	default:
		return fmt.Errorf("unknown type: %T", v)
	}
}

func hashMessage(h hash.Hash, m protoreflect.Message) error {
	if m.Descriptor().FullName() == anyName {
		a, err := m.Interface().(*anypb.Any).UnmarshalNew()
		if err != nil {
			return err
		}
		return hashMessage(h, a.ProtoReflect())
	}
	if m.GetUnknown() != nil {
		return fmt.Errorf("unknown fields cannot be hashed")
	}
	// Collect a sorted list of populated message fields.
	var fds []protoreflect.FieldDescriptor
	m.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		fds = append(fds, fd)
		return true
	})
	sort.Slice(fds, func(i, j int) bool { return fds[i].Number() < fds[j].Number() })
	// Iterate over message fields.
	for _, fd := range fds {
		if err := hashNumber(h, uint64(fd.Number())); err != nil {
			return err
		}
		if err := hashValue(h, m.Get(fd)); err != nil {
			return err
		}
	}
	return nil
}

func hashList(h hash.Hash, lv protoreflect.List) error {
	if err := hashNumber(h, uint64(lv.Len())); err != nil {
		return err
	}
	for i := range lv.Len() {
		if err := hashValue(h, lv.Get(i)); err != nil {
			return err
		}
	}
	return nil
}

func hashMap(h hash.Hash, mv protoreflect.Map) error {
	if err := hashNumber(h, uint64(mv.Len())); err != nil {
		return err
	}
	if mv.Len() == 0 {
		return nil
	}

	// Collect a sorted list of populated map entries.
	var ks []protoreflect.MapKey
	mv.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		ks = append(ks, k)
		return true
	})

	var sortFn func(i, j int) bool
	switch ks[0].Interface().(type) {
	case bool:
		sortFn = func(i, j int) bool { return !ks[i].Bool() && ks[j].Bool() }
	case int32, int64:
		sortFn = func(i, j int) bool { return ks[i].Int() < ks[j].Int() }
	case uint32, uint64:
		sortFn = func(i, j int) bool { return ks[i].Uint() < ks[j].Uint() }
	case string:
		sortFn = func(i, j int) bool { return ks[i].String() < ks[j].String() }
	default:
		return errors.New("invalid map key type")
	}
	sort.Slice(ks, sortFn)

	// Iterate over map entries.
	for _, k := range ks {
		if err := hashValue(h, k.Value()); err != nil {
			return err
		}
		if err := hashValue(h, mv.Get(k)); err != nil {
			return err
		}
	}
	return nil
}

func hashNumber(h hash.Hash, v uint64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	if _, err := h.Write(b[:]); err != nil {
		return err
	}
	return nil
}

func hashBytes(h hash.Hash, v []byte) error {
	if err := hashNumber(h, uint64(len(v))); err != nil {
		return err
	}
	if _, err := h.Write([]byte(v)); err != nil {
		return err
	}
	return nil
}
