// Copyright 2020 The LUCI Authors.
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

package datastore

import (
	"encoding/binary"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/internal/zstd"
)

// protoOption specifies how to handle field implementing proto.Message.
//
// **Modern format**: first byte reserved for denoting format and is thus not
// compatible with legacy format. Options supported currently:
//
//	"nocompress"   - (same as default) no compression.
//	                 Able to read compressed items in modern format.
//	"zstd"          - compress serialized proto with zstd encoding.
//	                 Able to read non-compressed items in modern format.
//
// **Legacy formats**: not compatible between each other or modern formats.
// Options supported:
//
//	"legacy"       - reads/writes serialized proto message. Useful for
//	                 migrating off `proto-gae` tool.
type protoOption string

const (
	// non-legacy proto serialization first writes a varint with its kind.
	// To avoid accidental overlap with legacy protobuf encoding and ensure
	// that proto unmarshaling will error out on it, use
	//
	//     number := (N<<3) | 4
	//
	// Explanation:
	// Proto serialization also first writes a varint on the wire representing so
	// called "tag", which is comprised of field number and wire type (see [1]):
	//     tag := (field_number<<3) | wire_type
	//
	// There are 2 long deprecated wire type which isn't even supported by most
	// languages (see [2]), one of which is "group end" which has a value of 4.
	// Group end specifically shouldn't be at the beginning of a message,
	// notwithstanding smart-ass hackery like this one, of course.
	// Therefore, for any field number N, value of `(N<<3) | 4`, incorrect proto
	// decoding will error out pretty quickly.
	//
	// [1] https://developers.google.com/protocol-buffers/docs/encoding#structure
	// [2] https://stackoverflow.com/a/33821387

	// WARNING: changing these values is not backwards compatible.

	protoBinOptNoCompress = (1 << 3) | 4
	protoBinOptZSTD       = (2 << 3) | 4
)

// compressionThreshold is the number of bytes of serialized proto value after which
// compression kicks in.
const compressionThreshold = 16 * 1024

var errInvalidProtoPrefix = fmt.Errorf("invalid gae proto serialization or unrecognized compression scheme")

func protoToProperty(pb proto.Message, opt protoOption) (prop Property, err error) {
	// proto can't marshall to io.Writer, so might as well serialize it now,
	// but leave first byte free for "nocompress" case.
	blob := make([]byte, 1, 16)
	if blob, err = (proto.MarshalOptions{}).MarshalAppend(blob, pb); err != nil {
		return
	}
	pbblob := blob[1:]

	if opt == "" /*default*/ {
		opt = "nocompress"
		if len(pbblob) > compressionThreshold {
			opt = "zstd"
		}
	}

	switch opt {
	case "legacy":
		prop = MkPropertyNI(pbblob)
		return
	case "nocompress":
		write1ByteProtoOpt(blob, protoBinOptNoCompress)
		prop = MkPropertyNI(blob)
		return
	case "zstd":
		// allocate new buffer for compressed data, hoping for ~2x compression.
		blob = make([]byte, 1, max(1, len(pbblob)/2))
		write1ByteProtoOpt(blob, protoBinOptZSTD)
		blob = zstd.EncodeAll(pbblob, blob)
		prop = MkPropertyNI(blob)
		return
	default:
		panic(fmt.Errorf("unrecognized proto option: %q", opt))
	}
}

func protoFromProperty(field reflect.Value, prop Property, opt protoOption) error {
	// Python can write NULL properties for empty blobs. Treat them as missing.
	if prop.Type() == PTNull {
		field.SetZero()
		return nil
	}

	pm, _ := field.Interface().(proto.Message)
	data, err := prop.Project(PTBytes)
	if err != nil {
		return err
	}
	blob := data.([]byte)
	pm = pm.ProtoReflect().New().Interface()

	switch opt {
	case "legacy":
		break // read entire blob.
	case "zstd", "", "nocompress":
		switch binOpt, readBytes := binary.Uvarint(blob); {
		case readBytes != 1:
			return errInvalidProtoPrefix
		case protoBinOptNoCompress == binOpt:
			blob = blob[1:]
		case protoBinOptZSTD == binOpt:
			if blob, err = zstd.DecodeAll(blob[1:], nil); err != nil {
				return err
			}
		default:
			return errInvalidProtoPrefix
		}
	default:
		panic(fmt.Errorf("unrecognized proto option: %q", opt))
	}

	if err = proto.Unmarshal(blob, pm); err != nil {
		return err
	}
	field.Set(reflect.ValueOf(pm))
	return nil
}

func write1ByteProtoOpt(b []byte, opt uint64) {
	if n := binary.PutUvarint(b, opt); n != 1 {
		panic(fmt.Errorf("protoOption longer than 1 byte: %d", n))
	}
}
