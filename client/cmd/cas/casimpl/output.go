// Copyright 2021 The LUCI Authors.
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

package casimpl

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io/ioutil"
	"sort"

	"go.chromium.org/luci/common/errors"
)

type StatusCode int

// possible exit errors
const (
	// Bad command line arguments were provided
	ArgumentsInvalid StatusCode = iota
	// Error authenticating with RBE-CAS server
	AuthenticationError
	// Error instantiating a new RBE client
	ClientError
	// The provided digest is bad and the file/dir cannot be found
	DigestInvalid
	// Disk I/O issues
	IOError
	// Issue with RPC to RBE-CAS server
	RPCError
	// Any uncategorised error
	Unknown
)

// writeExitResult writes the status msg
func writeExitResult(path string, statusCode StatusCode) error {
	if path == "" {
		return nil
	}

	var toString = map[StatusCode]string{
		ArgumentsInvalid:    "arguments_invalid",
		AuthenticationError: "authentication_error",
		ClientError:         "client_error",
		DigestInvalid:       "digest_invalid",
		IOError:             "io_error",
		RPCError:            "rpc_error",
		Unknown:             "unknown",
	}

	body, err := json.Marshal(struct {
		Result string `json:"result"`
	}{
		Result: toString[statusCode],
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal json").Err()
	}
	if err := ioutil.WriteFile(path, body, 0600); err != nil {
		return errors.Annotate(err, "failed to write json").Err()
	}
	return nil
}

// writeStats writes cache stats in packed format.
func writeStats(path string, hot, cold []int64) error {
	// Copy before sort.
	cold = append([]int64{}, cold...)
	hot = append([]int64{}, hot...)

	sort.Slice(cold, func(i, j int) bool { return cold[i] < cold[j] })
	sort.Slice(hot, func(i, j int) bool { return hot[i] < hot[j] })

	packedCold, err := pack(cold)
	if err != nil {
		return errors.Annotate(err, "failed to pack uploaded items").Err()
	}

	packedHot, err := pack(hot)
	if err != nil {
		return errors.Annotate(err, "failed to pack not uploaded items").Err()
	}

	statsJSON, err := json.Marshal(struct {
		ItemsCold []byte `json:"items_cold"`
		ItemsHot  []byte `json:"items_hot"`
		Result    string `json:"result"`
	}{
		ItemsCold: packedCold,
		ItemsHot:  packedHot,
		Result:    "success",
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal json").Err()
	}
	if err := ioutil.WriteFile(path, statsJSON, 0600); err != nil {
		return errors.Annotate(err, "failed to write json").Err()
	}

	return nil
}

// pack returns a deflate'd buffer of delta encoded varints.
func pack(values []int64) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if values[0] < 0 {
		return nil, errors.Reason("values must be between 0 and 2**63").Err()
	}
	if values[len(values)-1] < 0 {
		return nil, errors.Reason("values must be between 0 and 2**63").Err()
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	var last int64
	for _, value := range values {
		v := value
		value -= last
		if value < 0 {
			return nil, errors.Reason("list must be sorted ascending").Err()
		}
		last = v
		for value > 127 {
			if _, err := w.Write([]byte{byte(1<<7 | value&0x7f)}); err != nil {
				return nil, errors.Annotate(err, "failed to write").Err()
			}
			value >>= 7
		}
		if _, err := w.Write([]byte{byte(value)}); err != nil {
			return nil, errors.Annotate(err, "failed to write").Err()
		}

	}
	if err := w.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to close zlib writer").Err()
	}

	return b.Bytes(), nil
}

// unpack decompresses a deflate'd delta encoded list of varints.
func unpack(data []byte) ([]int64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var ret []int64
	var value int64
	var base int64 = 1
	var last int64

	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get zlib reader").Err()
	}
	defer r.Close()

	data, err = ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read all").Err()
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
