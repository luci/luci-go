// Copyright 2019 The LUCI Authors.
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

// Package utils implements an utility functions for running isolated file.
package utils

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
)

func Pack(values []int64) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if values[0] < 0 {
		return nil, errors.New("values must be between 0 and 2**63")
	}
	if values[len(values)-1] < 0 {
		return nil, errors.New("values must be between 0 and 2**63")
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	var last int64
	for _, value := range values {
		value -= last
		if value < 0 {
			return nil, errors.New("list must be sorted ascending")
		}
		last = value
		for value > 127 {
			w.Write([]byte{byte(1<<7 | value&0x7f)})
			value >>= 7
		}
		w.Write([]byte{byte(value)})

	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zlib writer: %v", err)
	}

	return b.Bytes(), nil
}

func Unpack(data []byte) ([]int64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var ret []int64
	var value int64
	var base int64 = 1
	var last int64

	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to get zlib reader: %v", err)
	}
	defer r.Close()

	data, err = ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read all: %v", err)
	}

	for _, valByte := range data {
		value += int64(valByte&0x7f) * base
		if valByte&0x80 > 0 {
			base <<= 7
			continue
		}
		ret = append(ret, value+last)
		last += value
		value = 0
		base = 1
	}

	return ret, nil
}
