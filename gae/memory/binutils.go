// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/luci/luci-go/common/funnybase"
)

func writeString(buf *bytes.Buffer, s string) {
	funnybase.WriteUint(buf, uint64(len(s)))
	buf.WriteString(s)
}

func readString(buf *bytes.Buffer) (string, error) {
	b, err := readBytes(buf)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeBytes(buf *bytes.Buffer, b []byte) {
	funnybase.WriteUint(buf, uint64(len(b)))
	buf.Write(b)
}

func readBytes(buf *bytes.Buffer) ([]byte, error) {
	val, err := funnybase.ReadUint(buf)
	if err != nil {
		return nil, err
	}
	if val > 2*1024*1024 { // 2MB
		return nil, fmt.Errorf("readBytes: tried to read %d bytes (> 2MB)", val)
	}
	retBuf := make([]byte, val)
	n, _ := buf.Read(retBuf) // err is either io.EOF or nil for bytes.Buffer
	if uint64(n) != val {
		return nil, fmt.Errorf("readBytes: expected %d bytes but read %d", val, n)
	}
	return retBuf, err
}

func writeFloat64(buf *bytes.Buffer, v float64) {
	// byte-ordered floats http://stereopsis.com/radix.html
	bits := math.Float64bits(v)
	bits = bits ^ (-(bits >> 63) | (1 << 63))
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, bits)
	buf.Write(data)
}

func readFloat64(buf *bytes.Buffer) (float64, error) {
	// byte-ordered floats http://stereopsis.com/radix.html
	data := make([]byte, 8)
	_, err := buf.Read(data)
	if err != nil {
		return 0, err
	}
	bits := binary.BigEndian.Uint64(data)
	return math.Float64frombits(bits ^ (((bits >> 63) - 1) | (1 << 63))), nil
}
