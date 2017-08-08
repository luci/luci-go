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

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
)

func encodeItemValue(pm ds.PropertyMap) []byte {
	pm, _ = pm.Save(false)

	buf := bytes.Buffer{}
	// errs can't happen, since we're using a byte buffer.
	_ = buf.WriteByte(byte(NoCompression))
	_ = serialize.WritePropertyMap(&buf, serialize.WithoutContext, pm)

	data := buf.Bytes()
	if buf.Len() > CompressionThreshold {
		buf2 := bytes.NewBuffer(make([]byte, 0, len(data)))
		_ = buf2.WriteByte(byte(ZlibCompression))
		writer := zlib.NewWriter(buf2)
		_, _ = writer.Write(data[1:]) // skip the NoCompression byte
		writer.Close()
		data = buf2.Bytes()
	}

	return data
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

	if CompressionType(compTypeByte) == ZlibCompression {
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
	return serialize.ReadPropertyMap(buf, serialize.WithoutContext, kc)
}
