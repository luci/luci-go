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
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// protoOption specifies how to handle field implementing proto.Message.
//
// **Modern format**: first byte reserved for denoting format and is thus not
// compatible with legacy format. By default, compresses serialized proto.
// Options supported currently:
//     "compress"     - (same as default) compress serialized proto with zlib.
//                      Able to read non-compressed items in modern format.
//     "nocompress"   - no compression.
//                      Able to read compressed items in modern format.
//
// **Legacy formats**: not compatible between each other or modern formats.
// Options supported:
//     "legacy"       - reads/writes serialized proto message. Useful for
//                      migrating off `proto-gae` tool.
//
type protoOption string

const (
	// non-legacy proto serialization first writes a varint with its kind.
	// To avoid accidental overlap with legacy protobuf encoding, use
	//     number := (N<<3) | 3
	//
	// Explanation:
	// Reason:
	// Proto serialization also first writes a varint on the wire representing so
	// called "tag", which is comprised of field number and wire type (see [1]):
	//     tag := (field_number<<3) | wire_type
	//
	// There are 2 long deprecated wire type which isn't even supported by most
	// languages (see [2]), one of which is "group start" which has a value of 3.
	// Therefore, for any field number N, value of `(N<<3) | 3` is highly unlikely
	// to be found as first data in a serialized protobuf.
	//
	// [1] https://developers.google.com/protocol-buffers/docs/encoding#structure
	// [2] https://stackoverflow.com/a/33821387

	// WARNING: changing these values is not backwards compatible.

	protoBinOptZlib       = (1 << 3) | 3
	protoBinOptNoCompress = (2 << 3) | 3
)

var errInvalidProtoPrefix = fmt.Errorf("invalid gae proto serialization")

func protoToProperty(pb proto.Message, opt protoOption) (prop Property, err error) {
	blob := make([]byte, 1, 16)
	// proto can't marshall to io.Writer, so might as well serialize it now,
	// but leave first byte free for "nocompress" case.
	if blob, err = (proto.MarshalOptions{}).MarshalAppend(blob, pb); err != nil {
		return
	}
	pbblob := blob[1:]

	switch opt {
	case "legacy":
		prop = MkPropertyNI(pbblob)
		return
	case "nocompress":
		write1ByteProtoOpt(blob, protoBinOptNoCompress)
		prop = MkPropertyNI(blob)
		return
	case "compress":
		// allocate new buffer for compressed data.
		blob = make([]byte, 1, 16)
		write1ByteProtoOpt(blob, protoBinOptZlib)
		buf := bytes.NewBuffer(blob)
		// TODO(tandrii): support custom compression level if needed.
		z := zlib.NewWriter(buf)
		if _, err = z.Write(pbblob); err != nil {
			_ = z.Close()
			return
		}
		if err = z.Close(); err != nil {
			return
		}
		prop = MkPropertyNI(buf.Bytes())
		return
	default:
		panic(fmt.Errorf("unrecognized proto option: %q", opt))
	}
}

func protoFromProperty(field reflect.Value, prop Property, opt protoOption) error {
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
	case "compress", "", "nocompress":
		switch binOpt, readBytes := binary.Uvarint(blob); {
		case readBytes != 1:
			return errInvalidProtoPrefix
		default:
			return errInvalidProtoPrefix
		case protoBinOptNoCompress == binOpt:
			blob = blob[readBytes:]
		case protoBinOptZlib == binOpt:
			if blob, err = zlibDecompress(blob[readBytes:]); err != nil {
				return err
			}
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

func zlibDecompress(compressed []byte) (plain []byte, err error) {
	var z io.ReadCloser
	z, err = zlib.NewReader(bytes.NewBuffer(compressed))
	if err != nil {
		return
	}
	defer func() { err = z.Close() }()
	plain, err = ioutil.ReadAll(z)
	return
}

func write1ByteProtoOpt(b []byte, opt uint64) {
	if n := binary.PutUvarint(b, opt); n != 1 {
		panic(fmt.Errorf("protoOption longer than 1 byte: %d", n))
	}
}
