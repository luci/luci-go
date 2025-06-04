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
	"encoding/json"
	"os"
	"sort"

	"go.chromium.org/luci/common/data/packedintset"
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
func writeExitResult(path string, statusCode StatusCode, digest string) error {
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

	type ErrorDetails struct {
		Digest string `json:"digest,omitempty"`
	}

	var body struct {
		Result       string       `json:"result"`
		ErrorDetails ErrorDetails `json:"error_details,omitempty"`
	}

	body.Result = toString[statusCode]
	if digest != "" {
		body.ErrorDetails = ErrorDetails{Digest: digest}
	}

	out, err := json.Marshal(body)
	if err != nil {
		return errors.Fmt("failed to marshal json: %w", err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return errors.Fmt("failed to write json: %w", err)
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

	var sizeCold, sizeHot int64
	for _, fileSize := range cold {
		sizeCold += fileSize
	}
	for _, fileSize := range hot {
		sizeHot += fileSize
	}

	packedCold, err := packedintset.Pack(cold)
	if err != nil {
		return errors.Fmt("failed to pack uploaded items: %w", err)
	}

	packedHot, err := packedintset.Pack(hot)
	if err != nil {
		return errors.Fmt("failed to pack not uploaded items: %w", err)
	}

	statsJSON, err := json.Marshal(struct {
		ItemsCold []byte `json:"items_cold"`
		SizeCold  int64  `json:"size_cold"`
		ItemsHot  []byte `json:"items_hot"`
		SizeHot   int64  `json:"size_hot"`
		Result    string `json:"result"`
	}{
		ItemsCold: packedCold,
		SizeCold:  sizeCold,
		ItemsHot:  packedHot,
		SizeHot:   sizeHot,
		Result:    "success",
	})
	if err != nil {
		return errors.Fmt("failed to marshal json: %w", err)
	}
	if err := os.WriteFile(path, statsJSON, 0600); err != nil {
		return errors.Fmt("failed to write json: %w", err)
	}

	return nil
}
