// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"

	ds "github.com/luci/gae/service/datastore"
)

func encodeItemValue(pm ds.PropertyMap) []byte {
	pm, _ = pm.Save(false)

	buf := bytes.Buffer{}
	// errs can't happen, since we're using a byte buffer.
	_ = buf.WriteByte(byte(NoCompression))
	_ = pm.Write(&buf, ds.WithoutContext)

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

func decodeItemValue(val []byte, ns, aid string) (ds.PropertyMap, error) {
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
	ret := ds.PropertyMap{}
	err = ret.Read(buf, ds.WithoutContext, ns, aid)
	return ret, err
}
