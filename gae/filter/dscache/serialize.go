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
	"io"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gae/internal/zstd"
	ds "go.chromium.org/luci/gae/service/datastore"
)

// UseZstd, if true, instructs the dscache to use zstd algorithm for compression
// instead of zlib.
//
// Already compressed zlib entities are supported even when UseZstd is true.
var UseZstd = false

type compressionType byte

const (
	compressionNone compressionType = 0
	compressionZlib compressionType = 1
	compressionZstd compressionType = 2
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

	// Use zlib in legacy applications that didn't opt-in into using zstd.
	algo := compressionZlib
	if UseZstd {
		algo = compressionZstd
	}

	// Compress into the new buffer.
	buf2.Write(pfx)
	buf2.WriteByte(byte(algo))
	switch algo {
	case compressionZlib:
		writer := zlib.NewWriter(buf2)
		writer.Write(data)
		writer.Close()
		return buf2.Bytes(), nil
	case compressionZstd:
		return zstd.EncodeAll(data, buf2.Bytes()), nil
	default:
		panic("impossible")
	}
}

func decodeItemValue(val []byte, kc ds.KeyContext) (ds.PropertyMap, error) {
	if len(val) == 0 {
		return nil, ds.ErrNoSuchEntity
	}
	if len(val) < 1 {
		return nil, errors.Reason("missing the compression type byte").Err()
	}

	compTypeByte, data := val[0], val[1:]

	switch compressionType(compTypeByte) {
	case compressionNone:
		// already decompressed

	case compressionZlib:
		reader, err := zlib.NewReader(bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
		if data, err = io.ReadAll(reader); err != nil {
			return nil, err
		}
		if err = reader.Close(); err != nil {
			return nil, err
		}

	case compressionZstd:
		var err error
		data, err = zstd.DecodeAll(data, nil)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.Reason("unknown compression scheme #%d", compTypeByte).Err()
	}

	return ds.Deserializer{KeyContext: kc}.PropertyMap(bytes.NewBuffer(data))
}
