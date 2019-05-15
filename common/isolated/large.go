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

package isolated

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
)

// Pack returns a deflate'd buffer of delta encoded varints.
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
		v := value
		value -= last
		if value < 0 {
			return nil, errors.New("list must be sorted ascending")
		}
		last = v
		for value > 127 {
			if _, err := w.Write([]byte{byte(1<<7 | value&0x7f)}); err != nil {
				return nil, fmt.Errorf("failed to write: %v", err)
			}
			value >>= 7
		}
		if _, err := w.Write([]byte{byte(value)}); err != nil {
			return nil, fmt.Errorf("failed to write: %v", err)
		}

	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zlib writer: %v", err)
	}

	return b.Bytes(), nil
}

// Unpack decompresses a deflate'd delta encoded list of varints.
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
