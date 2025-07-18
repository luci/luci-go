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
	"io"
	"math"
	"reflect"
	"sort"

	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/reflectutil"
)

// internal type for marshalMessage
//
// This maps a field number to a value for that field.
//
// A fieldVal can either be `known` (i.e. is defined in the proto)
// or `unknown` (i.e. field was present in an unmarshaled msgpackpb
// message). Exactly one of `(fd, v)` or `raw` will be set.
type fieldVal struct {
	n int32 // the proto field tag number

	// set if field was `known`
	fd protoreflect.FieldDescriptor
	v  protoreflect.Value

	// set if field was `unknown`
	raw msgpack.RawMessage
}

func (o *options) marshalValue(enc *msgpack.Encoder, fd protoreflect.FieldDescriptor, val protoreflect.Value) error {
	kind := fd.Kind()
	if fd.IsMap() {
		kind = fd.MapValue().Kind()
	}

	switch kind {
	case protoreflect.BoolKind:
		// note: this should only ever encode `true`, because proto range should
		// skip it if it's false.
		return enc.EncodeBool(val.Bool())

	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return enc.EncodeInt(val.Int())

	case protoreflect.EnumKind:
		return enc.EncodeInt(int64(val.Enum()))

	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return enc.EncodeUint(val.Uint())

	case protoreflect.FloatKind:
		// this mimics lua's handling of floats-containing-integers

		// convert to float32 here is potentially lossy, so we do it before
		// math.Floor. Conversion from float32 to float64 is NOT lossy.
		f := float32(val.Float())
		if math.Floor(float64(f)) == float64(f) {
			return enc.EncodeInt(int64(f))
		}
		return enc.EncodeFloat32(f)

	case protoreflect.DoubleKind:
		// this mimics lua's handling of floats-containing-integers
		f := val.Float()
		if math.Floor(f) == f {
			return enc.EncodeInt(int64(f))
		}
		return enc.EncodeFloat64(f)

	case protoreflect.StringKind:
		sVal := val.String()
		if ival, ok := o.internMarshalTable[sVal]; ok {
			return enc.EncodeUint(uint64(ival))
		}

		return enc.EncodeString(sVal)

	case protoreflect.MessageKind:
		return o.marshalMessage(enc, val.Message())
	}
	return errors.Fmt("marshalValue: invalid kind %q", kind)
}

func (o *options) appendRawMsgpackMsg(raw []byte, to *[]fieldVal, tf takenFields) error {
	dec := msgpack.GetDecoder()
	defer func() {
		if dec != nil {
			msgpack.PutDecoder(dec)
		}
	}()

	dec.Reset(bytes.NewReader(raw))
	dec.SetMapDecoder((*msgpack.Decoder).DecodeTypedMap)

	msgItemLen, nextKey, err := getMapLen(dec)
	if err != nil {
		return errors.Fmt("expected message length: %w", err)
	}

	for i := range msgItemLen {
		tag, err := getNextMsgTag(dec, nextKey)
		if err != nil {
			return errors.Fmt("reading message %d'th tag: %w", i, err)
		}
		if err = tf.add(tag); err != nil {
			return errors.Fmt("reading message %d'th tag: %w", i, err)
		}

		var rawVal msgpack.RawMessage
		if o.deterministic {
			var valI any
			valI, err = dec.DecodeInterfaceLoose()
			if err == nil {
				rawVal, err = msgpackpbDeterministicEncode(reflect.ValueOf(valI))
			}
		} else {
			rawVal, err = dec.DecodeRaw()
		}
		if err != nil {
			return errors.Fmt("reading message %d't field: %w", i, err)
		}

		*to = append(*to, fieldVal{
			n:   tag,
			raw: rawVal,
		})
	}

	return nil
}

type takenFields map[int32]struct{}

func (t takenFields) add(tag int32) error {
	if tag == 0 {
		return errors.New("invalid tag 0")
	}

	if _, ok := t[tag]; ok {
		return errors.Fmt("duplicate tag %d", tag)
	}
	t[tag] = struct{}{}

	return nil
}

func (o *options) marshalMessage(enc *msgpack.Encoder, msg protoreflect.Message) (err error) {
	tf := takenFields{}
	populatedFields := make([]fieldVal, 0, msg.Descriptor().Fields().Len())
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fv := fieldVal{fd: fd, v: v}
		fv.n = int32(fd.Number())
		if err := tf.add(fv.n); err != nil {
			panic(errors.Fmt("impossible: %w", err))
		}
		populatedFields = append(populatedFields, fv)
		return true
	})

	unknownFieldsRaw := msg.GetUnknown()
	if len(unknownFieldsRaw) > 0 {
		if o.unknownFieldBehavior == disallowUnknownFields {
			return errors.New("message has unknown fields")
		}

		var uf UnknownFields
		if err := proto.Unmarshal(unknownFieldsRaw, &uf); err != nil {
			return errors.New("unmarshaling unknown msgpack fields")
		}
		if len(uf.ProtoReflect().GetUnknown()) > 0 {
			return errors.New("unknown non-msgpack fields unsupported")
		}

		if o.unknownFieldBehavior == preserveUnknownFields {
			if err := o.appendRawMsgpackMsg(uf.MsgpackpbData, &populatedFields, tf); err != nil {
				return errors.New("parsing unknown fields")
			}
		}
	}

	encodeLen := func() error {
		return enc.EncodeMapLen(len(populatedFields))
	}
	encodeKey := func(fv *fieldVal) error {
		return enc.EncodeInt(int64(fv.n))
	}

	if o.deterministic {
		sort.Slice(populatedFields, func(i, j int) bool { return populatedFields[i].n < populatedFields[j].n })
		count := int32(len(populatedFields))
		if count > 0 && populatedFields[0].n == 1 && populatedFields[len(populatedFields)-1].n == count {
			encodeLen = func() error {
				return enc.EncodeArrayLen(int(count))
			}
			encodeKey = func(fv *fieldVal) error { return nil }
		}
	}

	if err := encodeLen(); err != nil {
		return err
	}
	for _, fv := range populatedFields {
		if err := encodeKey(&fv); err != nil {
			return err
		}

		if len(fv.raw) > 0 {
			if err := enc.Encode(fv.raw); err != nil {
				return err
			}
			continue
		}

		fd := fv.fd
		name := fd.Name()

		// list[*]
		if fd.IsList() {
			lst := fv.v.List()
			if err := enc.EncodeArrayLen(lst.Len()); err != nil {
				return err
			}
			for i := range lst.Len() {
				if err := o.marshalValue(enc, fd, lst.Get(i)); err != nil {
					return errors.Fmt("%s[%d]: %w", name, i, err)
				}
			}
			continue
		}

		// map[simple]*
		if fd.IsMap() {
			m := fv.v.Map()
			if err := enc.EncodeMapLen(m.Len()); err != nil {
				return err
			}
			rangeFn := m.Range
			if o.deterministic {
				rangeFn = func(f func(protoreflect.MapKey, protoreflect.Value) bool) {
					reflectutil.MapRangeSorted(m, fd.MapKey().Kind(), f)
				}
			}
			var encodeKey func(protoreflect.MapKey) error
			if len(o.internMarshalTable) > 0 && fd.MapKey().Kind() == protoreflect.StringKind {
				encodeKey = func(mk protoreflect.MapKey) error {
					sval := mk.String()
					if ival, ok := o.internMarshalTable[sval]; ok {
						if err := enc.EncodeUint(uint64(ival)); err != nil {
							return err
						}
						return nil
					}
					return enc.EncodeString(sval)
				}
			} else {
				encodeKey = func(mk protoreflect.MapKey) error {
					return enc.Encode(mk.Interface())
				}
			}
			rangeFn(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
				if err = encodeKey(mk); err == nil {
					err = o.marshalValue(enc, fd, v)
				}
				err = errors.WrapIf(err, "%s[%s]", name, mk)
				return err == nil
			})
			if err != nil {
				return err
			}
			continue
		}

		if err := o.marshalValue(enc, fd, fv.v); err != nil {
			return errors.Fmt("%s: %w", name, err)
		}
	}

	return
}

// MarshalStream is like Marshal but outputs to an io.Writer instead of
// returning a string.
func MarshalStream(writer io.Writer, msg proto.Message, opts ...Option) error {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}

	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)

	enc.Reset(writer)
	enc.UseCompactInts(true)
	enc.UseCompactFloats(true)
	err := o.marshalMessage(enc, msg.ProtoReflect())

	return err
}

// Marshal encodes all the known fields in msg to a msgpack string.
//
// By default, this will emit any unknown msgpack fields (generated by the
// Unmarshal method in this package) back to the serialized message. Pass
// IgnoreUnknownFields or DisallowUnknownFields to affect this behavior.
//
// This can also produce a deterministic encoding if Deterministic is passed as
// an option. Otherwise this will do a faster non-determnistic encoding without
// trying to sort field tags or map keys.
//
// Returns an error if `msg` contains unknown fields.
func Marshal(msg proto.Message, opts ...Option) (msgpack.RawMessage, error) {
	ret := bytes.Buffer{}
	err := MarshalStream(&ret, msg, opts...)
	return ret.Bytes(), err
}
