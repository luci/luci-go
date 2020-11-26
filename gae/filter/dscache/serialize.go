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

package dscache

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"

	ds "go.chromium.org/luci/gae/service/datastore"
)

type compressionType byte

const (
	compressionNone compressionType = iota
	compressionZlib
)

func encodeItemValue(pm ds.PropertyMap, pfx []byte) ([]byte, error) {
	var err error
	if pm, err = pm.Save(false); err != nil {
		return nil, err
	}

	// Try to write as uncompressed first. Capacity of 256 is picked arbitrarily.
	// Most entities are pretty small.
	buf := bytes.NewBuffer(make([]byte, 0, 256))
	buf.Write(pfx)
	buf.WriteByte(byte(compressionNone))
	if err := ds.Serialize.PropertyMap(buf, pm); err != nil {
		return nil, err
	}

	// If it is small enough, we are done.
	if buf.Len() <= CompressionThreshold {
		return buf.Bytes(), nil
	}

	// If too big, grab a new buffer and compress data there. Preallocate a new
	// buffer assuming 2x compression.
	data := buf.Bytes()[len(pfx)+1:] // skip pfx and compressionNone byte
	buf2 := bytes.NewBuffer(make([]byte, 0, len(data)/2))

	// Compress into the new buffer.
	buf2.Write(pfx)
	buf2.WriteByte(byte(compressionZlib))
	writer := zlib.NewWriter(buf2)
	writer.Write(data)
	writer.Close()

	return buf2.Bytes(), nil
}

func decodeItemValue(val []byte, kc ds.KeyContext) (ds.PropertyMap, error) {
	if len(val) == 0 {
		return nil, ds.ErrNoSuchEntity
	}
	buf := bytes.NewBuffer(val)
	compTypeByte, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	if compressionType(compTypeByte) == compressionZlib {
		reader, err := zlib.NewReader(buf)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(data)
	}
	return ds.Deserializer{KeyContext: kc}.PropertyMap(buf)
}
