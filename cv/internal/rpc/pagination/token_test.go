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

package pagination

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPageTokens(t *testing.T) {
	t.Parallel()
	ctx := secrets.GeneratePrimaryTinkAEADForTest(context.Background())

	ftt.Run("Page token round trip", t, func(t *ftt.Test) {
		t.Run("not empty", func(t *ftt.Test) {
			// Any proto type should work.
			in := timestamppb.New(testclock.TestRecentTimeUTC)
			token, err := EncryptPageToken(ctx, in)
			assert.NoErr(t, err)
			out := &timestamppb.Timestamp{}
			assert.Loosely(t, DecryptPageToken(ctx, token, out), should.BeNil)
			assert.Loosely(t, out, should.Resemble(in))
		})
		t.Run("empty page token", func(t *ftt.Test) {
			token, err := EncryptPageToken(ctx, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, token, should.BeBlank)
			out := &timestamppb.Timestamp{}
			assert.Loosely(t, DecryptPageToken(ctx, token, out), should.BeNil)
			assert.Loosely(t, out, should.Resemble(&timestamppb.Timestamp{}))
		})
		t.Run("empty page token with typed nil", func(t *ftt.Test) {
			var typedNil *timestamppb.Timestamp
			token, err := EncryptPageToken(ctx, typedNil)
			assert.NoErr(t, err)
			assert.Loosely(t, token, should.BeBlank)
		})
	})

	ftt.Run("Bad page token", t, func(t *ftt.Test) {
		dst := &timestamppb.Timestamp{Seconds: 1, Nanos: 2}
		goodToken, err := EncryptPageToken(ctx, timestamppb.New(testclock.TestRecentTimeUTC))
		assert.NoErr(t, err)
		tokenBytes := []byte(goodToken)
		tokenBytes[10] = '\\'
		err = DecryptPageToken(ctx, string(tokenBytes), dst)
		assert.Loosely(t, err, should.ErrLike("illegal base64"))

		as, ok := appstatus.Get(err)
		assert.That(t, ok, should.BeTrue)
		assert.That(t, as, should.Resemble(status.New(codes.InvalidArgument, "invalid page token")))
	})
}
