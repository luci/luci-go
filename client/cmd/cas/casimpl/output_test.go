// Copyright 2026 The LUCI Authors.
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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractErrorCode(t *testing.T) {
	t.Parallel()

	ftt.Run(`extractErrorCode`, t, func(t *ftt.Test) {
		t.Run(`permission denied`, func(t *ftt.Test) {
			err := status.Error(codes.PermissionDenied, "unauthorized")
			code, d := extractErrorCode(err)
			assert.Loosely(t, code, should.Equal(AuthenticationError))
			assert.Loosely(t, d, should.BeEmpty)
		})

		t.Run(`not found with digest`, func(t *ftt.Test) {
			err := status.Error(codes.NotFound, "Digest e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/0 not found in the CAS")
			code, d := extractErrorCode(err)
			assert.Loosely(t, code, should.Equal(DigestInvalid))
			assert.Loosely(t, d, should.Equal("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/0"))
		})

		t.Run(`not found without digest in message`, func(t *ftt.Test) {
			err := status.Error(codes.NotFound, "item not found")
			code, d := extractErrorCode(err)
			assert.Loosely(t, code, should.Equal(DigestInvalid))
			assert.Loosely(t, d, should.BeEmpty)
		})

		t.Run(`internal RPC error`, func(t *ftt.Test) {
			err := status.Error(codes.Internal, "internal server failure")
			code, d := extractErrorCode(err)
			assert.Loosely(t, code, should.Equal(RPCError))
			assert.Loosely(t, d, should.BeEmpty)
		})

		t.Run(`non-grpc error`, func(t *ftt.Test) {
			err := errors.New("generic non-grpc error")
			code, d := extractErrorCode(err)
			assert.Loosely(t, code, should.Equal(RPCError))
			assert.Loosely(t, d, should.BeEmpty)
		})
	})
}

func TestWriteExitResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`writeExitResult`, t, func(t *ftt.Test) {
		t.Run(`empty path noop`, func(t *ftt.Test) {
			err := writeExitResult("", ClientError, "")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid path writes exit result json`, func(t *ftt.Test) {
			path := filepath.Join(t.TempDir(), "exit_result.json")
			err := writeExitResult(path, AuthenticationError, "foo/123")
			assert.Loosely(t, err, should.BeNil)

			data, err := os.ReadFile(path)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result       string `json:"result"`
				ErrorDetails struct {
					Digest string `json:"digest,omitempty"`
				} `json:"error_details,omitempty"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("authentication_error"))
			assert.Loosely(t, result.ErrorDetails.Digest, should.Equal("foo/123"))
		})
	})
}
