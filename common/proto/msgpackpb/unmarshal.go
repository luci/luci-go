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

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
)

func numericMapKey(key int32, kind protoreflect.Kind) (protoreflect.Value, error) {
	switch kind {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(key != 0), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(key), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(int64(key)), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(key)), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(key)), nil
	}

	return protoreflect.Value{}, errors.New("cannot convert numeric map key")
}

// unmarshalScalar will decode a value from the Decoder and return it as a Value,
// using arproximate protobuf decoding compatibility rules (i.e. Go numeric casts...
// official proto rules state that the casts should be "C++" style, and from my
// cursory read of the Golang spec, Go uses the same numeric conversion rules).
//
// NOTE: I considered the possibility where lua has encoded large int values
// with floats. However, inspecting the lua C msgpack library (all versions), it
// looks like it will already do the work to avoid using a float where possible.
// This means that if we get a float in a field which is supposed to have an
// integer type, we can treat it as a hard error.
func (o *options) unmarshalScalar(dec *msgpack.Decoder, fd protoreflect.FieldDescriptor) (ret protoreflect.Value, err error) {
	// DecodeInterfaceLoose will return:
	//   - int8, int16, and int32 are converted to int64,
	//   - uint8, uint16, and uint32 are converted to uint64,
	//   - float32 is converted to float64.
	//   - []byte is converted to string.
	val, err := dec.DecodeInterfaceLoose()
	if err != nil {
		err = errors.Fmt("decoding scalar: %w", err)
		return
	}

	switch fd.Kind() {
	case protoreflect.BoolKind:
		switch x := val.(type) {
		case bool:
			return protoreflect.ValueOfBool(x), nil
		case uint64:
			return protoreflect.ValueOfBool(x != 0), nil
		case int64:
			return protoreflect.ValueOfBool(x != 0), nil
		}

	case protoreflect.EnumKind:
		switch x := val.(type) {
		case bool:
			if x {
				return protoreflect.ValueOfEnum(1), nil
			}
			return protoreflect.ValueOfEnum(0), nil
		case uint64:
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(x)), nil
		case int64:
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(x)), nil
		}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		switch x := val.(type) {
		case bool:
			if x {
				return protoreflect.ValueOfInt32(1), nil
			}
			return protoreflect.ValueOfInt32(0), nil
		case uint64:
			return protoreflect.ValueOfInt32(int32(x)), nil
		case int64:
			return protoreflect.ValueOfInt32(int32(x)), nil
		}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		switch x := val.(type) {
		case bool:
			if x {
				return protoreflect.ValueOfInt64(1), nil
			}
			return protoreflect.ValueOfInt64(0), nil
		case uint64:
			return protoreflect.ValueOfInt64(int64(x)), nil
		case int64:
			return protoreflect.ValueOfInt64(x), nil
		}

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		switch x := val.(type) {
		case bool:
			if x {
				return protoreflect.ValueOfUint32(1), nil
			}
			return protoreflect.ValueOfUint32(0), nil
		case uint64:
			return protoreflect.ValueOfUint32(uint32(x)), nil
		case int64:
			return protoreflect.ValueOfUint32(uint32(x)), nil
		}

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		switch x := val.(type) {
		case bool:
			if x {
				return protoreflect.ValueOfUint64(1), nil
			}
			return protoreflect.ValueOfUint64(0), nil
		case uint64:
			return protoreflect.ValueOfUint64(x), nil
		case int64:
			return protoreflect.ValueOfUint64(uint64(x)), nil
		}

	case protoreflect.FloatKind:
		switch x := val.(type) {
		case uint64:
			// allowed, because lua will encode non-floatlike numbers as integers.
			return protoreflect.ValueOfFloat32(float32(x)), nil
		case int64:
			// allowed, because lua will encode non-floatlike negative numbers as integers.
			return protoreflect.ValueOfFloat32(float32(x)), nil
		case float32:
			return protoreflect.ValueOfFloat32(x), nil
		case float64:
			return protoreflect.ValueOfFloat32(float32(x)), nil
		}

	case protoreflect.DoubleKind:
		switch x := val.(type) {
		case uint64:
			// allowed, because lua will encode non-floatlike numbers as integers.
			return protoreflect.ValueOfFloat64(float64(x)), nil
		case int64:
			// allowed, because lua will encode non-floatlike negative numbers as integers.
			return protoreflect.ValueOfFloat64(float64(x)), nil
		case float32:
			return protoreflect.ValueOfFloat64(float64(x)), nil
		case float64:
			return protoreflect.ValueOfFloat64(x), nil
		}

	case protoreflect.StringKind, protoreflect.BytesKind:
		var checkIntern bool
		var internIdx int
		switch x := val.(type) {
		case string:
			return protoreflect.ValueOf(val), nil
		case uint64:
			checkIntern = true
			internIdx = int(x)
		case int64:
			checkIntern = true
			internIdx = int(x)
		}

		if checkIntern {
			if internIdx < len(o.internUnmarshalTable) {
				return protoreflect.ValueOfString(o.internUnmarshalTable[internIdx]), nil
			}
			err = errors.Fmt("interned string has index out of bounds: %d", internIdx)
			return
		}
	}

	err = errors.Fmt("bad type: expected %s, got %T", fd.Kind(), val)
	return
}

func isMap(dec *msgpack.Decoder) (bool, error) {
	c, err := dec.PeekCode()
	if err != nil {
		return false, err
	}

	if msgpcode.IsFixedMap(c) || c == msgpcode.Map16 || c == msgpcode.Map32 {
		return true, nil
	}
	return false, nil
}

// Because lua tables are used for both maps and lists, we can't reliably encode
// a map as a map, because if it HAPPENS to have numeric indexes which are all
// 1..N, cmsgpack will consider this to be a list and encode just the values in
// sequence. Fortunately, in this case, the list is guaranteed to be already
// sorted (by definition)!
func getMapLen(dec *msgpack.Decoder) (n int, nextKey func() int32, err error) {
	ism, err := isMap(dec)
	if err != nil {
		return
	}

	if ism {
		n, err = dec.DecodeMapLen()
		return
	}

	n, err = dec.DecodeArrayLen()
	var idx int32 // remember; lua indexes are 1 based, so we ++ and then return
	nextKey = func() int32 { idx++; return idx }
	return
}

func getNextMsgTag(dec *msgpack.Decoder, nextKey func() int32) (tag int32, err error) {
	if nextKey != nil {
		tag = nextKey()
	} else {
		if tag, err = dec.DecodeInt32(); err != nil {
			return
		}
	}
	return
}

func (o *options) unmarshalMessage(dec *msgpack.Decoder, to protoreflect.Message) error {
	msgItemLen, nextKey, err := getMapLen(dec)
	if err != nil {
		return errors.Fmt("expected message length: %w", err)
	}

	d := to.Descriptor()
	fieldsD := d.Fields()

	var unknownFields map[int32]msgpack.RawMessage

	for i := range msgItemLen {
		tag, err := getNextMsgTag(dec, nextKey)
		if err != nil {
			return errors.Fmt("reading message tag: %w", err)
		}

		fd := fieldsD.ByNumber(protowire.Number(tag))
		if fd == nil {
			switch o.unknownFieldBehavior {
			case ignoreUnknownFields:
				//pass
			case disallowUnknownFields:
				return errors.Fmt("unknown field tag %d on decoded field %d", tag, i)
			case preserveUnknownFields:
				if unknownFields == nil {
					unknownFields = map[int32]msgpack.RawMessage{}
				}
				if unknownFields[tag], err = dec.DecodeRaw(); err != nil {
					return errors.Fmt("unknown field tag %d on decoded field %d: cannot decode msgpack", tag, i)
				}
			default:
				panic("unknown value of o.unknownFieldBehavior")
			}
			continue
		}
		name := fd.Name()

		// now we check that the encoded thing is the thing we expect to find.
		if fd.IsList() {
			// note that if the input array was `sparse` (contained nil values), it MAY
			// be encoded as a map.
			ism, err := isMap(dec)
			if err != nil {
				return errors.Fmt("%s: expected list or map: %w", name, err)
			}

			lst := to.Mutable(fd).List()

			var mapLen int
			var decodeIdx func() (int, error)
			var addValue func(i int, v protoreflect.Value)
			var postProcess func()
			if ism {
				if mapLen, err = dec.DecodeMapLen(); err != nil {
					return errors.Fmt("%s: expected sparse list: %w", name, err)
				}

				maxIdx := 0
				decodeIdx = func() (int, error) {
					ret, err := dec.DecodeInt()
					if err != nil {
						return ret, err
					}
					if ret > maxIdx {
						maxIdx = ret
					}
					return ret, err
				}
				sparse := make(map[int]protoreflect.Value, mapLen)
				addValue = func(i int, v protoreflect.Value) { sparse[i] = v }
				zero := lst.NewElement()
				postProcess = func() {
					for i := 0; i <= maxIdx; i++ {
						if val, ok := sparse[i]; ok {
							lst.Append(val)
						} else {
							lst.Append(zero)
						}
					}
				}
			} else {
				if mapLen, err = dec.DecodeArrayLen(); err != nil {
					return errors.Fmt("%s: expected list: %w", name, err)
				}

				addValue = func(_ int, v protoreflect.Value) { lst.Append(v) }
				decodeIdx = func() (int, error) { return 0, nil }
			}

			for i := 0; i < mapLen; i++ {
				idx, err := decodeIdx()
				if err != nil {
					return errors.Fmt("%s[%d]: expected int key: %w", name, i, err)
				}

				var el protoreflect.Value
				if fd.Kind() == protoreflect.MessageKind {
					el = lst.NewElement()
					if err = o.unmarshalMessage(dec, el.Message()); err != nil {
						return errors.Fmt("%s[%d]: %w", name, i, err)
					}
				} else {
					if el, err = o.unmarshalScalar(dec, fd); err != nil {
						return errors.Fmt("%s[%d]: %w", name, i, err)
					}
				}
				addValue(idx, el)
			}
			if postProcess != nil {
				postProcess()
			}
			continue
		}

		if fd.IsMap() {
			mapLen, nextKey, err := getMapLen(dec)
			if err != nil {
				return errors.Fmt("%s: expected map: %w", name, err)
			}

			valFD := fd.MapValue()

			// ok, we're a map and they're a map, do the decode
			keyFD := fd.MapKey()
			mapp := to.Mutable(fd).Map()
			for i := range mapLen {
				var key protoreflect.Value
				if nextKey == nil {
					if key, err = o.unmarshalScalar(dec, keyFD); err != nil {
						return errors.Fmt("%s[idx:%d]: bad map key: %w", name, i, err)
					}
				} else {
					if key, err = numericMapKey(nextKey(), keyFD.Kind()); err != nil {
						return errors.Fmt("%s[idx:%d]: bad map key: %w", name, i, err)
					}
				}

				if valFD.Kind() == protoreflect.MessageKind {
					if err := o.unmarshalMessage(dec, mapp.Mutable(key.MapKey()).Message()); err != nil {
						return errors.Fmt("%s[%s]: %w", name, key, err)
					}
				} else {
					val, err := o.unmarshalScalar(dec, valFD)
					if err != nil {
						return errors.Fmt("%s[%s]: %w", name, key, err)
					}
					mapp.Set(key.MapKey(), val)
				}
			}
			continue
		}

		// singular field
		if fd.Kind() == protoreflect.MessageKind {
			if err := o.unmarshalMessage(dec, to.Mutable(fd).Message()); err != nil {
				return errors.Fmt("%s: %w", name, err)
			}
		} else {
			val, err := o.unmarshalScalar(dec, fd)
			if err != nil {
				return errors.Fmt("%s: %w", name, err)
			}
			to.Set(fd, val)
		}
	}

	if len(unknownFields) > 0 {
		unknownBuf := bytes.Buffer{}
		unknownEnc := msgpack.GetEncoder()
		defer msgpack.PutEncoder(unknownEnc)

		unknownEnc.Reset(&unknownBuf)
		unknownEnc.UseCompactFloats(true)
		unknownEnc.UseCompactInts(true)
		if err := unknownEnc.Encode(unknownFields); err != nil {
			panic(err)
		}
		protoEncUnknown, err := proto.Marshal(&UnknownFields{MsgpackpbData: unknownBuf.Bytes()})
		if err != nil {
			panic(err)
		}
		to.SetUnknown(protoEncUnknown)
	}
	return nil
}

// UnmarshalStream is like Unmarshal but takes an io.Reader instead of accepting
// a string.
//
// If the reader contains multiple msgpackpb messages, this function will stop
// exactly at where the next message in the stream begins (i.e. you could call
// this in a loop until the reader is exhausted to merge the messages together).
func UnmarshalStream(reader io.Reader, to proto.Message, opts ...Option) (err error) {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}

	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)

	dec.Reset(reader)

	return o.unmarshalMessage(dec, to.ProtoReflect())
}

// Unmarshal parses the encoded msgpack into the given proto message.
//
// This does NOT reset the Message; if it is partially populated, this will
// effectively do a proto.Merge on top of it.
//
// By default, this will output unknown fields in the Message, but this will
// only be usable by the corresponding Marshal function in this package. Pass
// IgnoreUnknownFields or DisallowUnknownFields to affect this behavior.
func Unmarshal(msg msgpack.RawMessage, to proto.Message, opts ...Option) (err error) {
	return UnmarshalStream(bytes.NewReader(msg), to, opts...)
}
