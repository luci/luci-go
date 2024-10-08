// Copyright 2022 The LUCI Authors.
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

// Package buildtoken provide related functions for generating and parsing build
// tokens.
package buildtoken

import (
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func mkToken(ctx context.Context, t testing.TB, buildID int64, purpose pb.TokenBody_Purpose) string {
	t.Helper()
	token, err := GenerateToken(ctx, buildID, purpose)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return token
}

func TestBuildToken(t *testing.T) {
	t.Parallel()

	ftt.Run("build token", t, func(t *ftt.Test) {
		secretStore := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"somekey": {Active: []byte("i r key")},
			},
		}
		ctx := secrets.Use(context.Background(), secretStore)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		t.Run("success (encrypted)", func(t *ftt.Test) {
			bID := int64(123)
			token := mkToken(ctx, t, bID, pb.TokenBody_BUILD)
			tBody, err := parseToTokenBodyImpl(ctx, token, 123, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody.BuildId, should.Equal(bID))
			assert.Loosely(t, tBody.Purpose, should.Equal(pb.TokenBody_BUILD))
			assert.Loosely(t, len(tBody.State), should.NotEqual(0))
		})

		t.Run("wrong build", func(t *ftt.Test) {
			bID := int64(123)
			token := mkToken(ctx, t, bID, pb.TokenBody_BUILD)
			_, err := parseToTokenBodyImpl(ctx, token, 321, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.ErrLike("token is for build 123"))
		})

		t.Run("skip build check", func(t *ftt.Test) {
			bID := int64(123)
			token := mkToken(ctx, t, bID, pb.TokenBody_BUILD)
			tBody, err := parseToTokenBodyImpl(ctx, token, 0, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody, should.NotBeNil)
		})

		t.Run("skip purpose check", func(t *ftt.Test) {
			bID := int64(123)
			tBody, err := parseToTokenBodyImpl(ctx, mkToken(ctx, t, bID, pb.TokenBody_BUILD), bID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody, should.NotBeNil)

			tBody, err = parseToTokenBodyImpl(ctx, mkToken(ctx, t, bID, pb.TokenBody_TASK), bID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody, should.NotBeNil)
		})

		t.Run("multi purpose check", func(t *ftt.Test) {
			bID := int64(123)
			token := mkToken(ctx, t, bID, pb.TokenBody_BUILD)
			tBody, err := parseToTokenBodyImpl(ctx, token, bID, pb.TokenBody_TASK, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody, should.NotBeNil)

			tBody, err = parseToTokenBodyImpl(ctx, token, bID, pb.TokenBody_BUILD, pb.TokenBody_TASK)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tBody, should.NotBeNil)
		})

		t.Run("wrong purpose", func(t *ftt.Test) {
			bID := int64(123)
			token := mkToken(ctx, t, bID, pb.TokenBody_BUILD)
			_, err := parseToTokenBodyImpl(ctx, token, 123, pb.TokenBody_TASK)
			assert.Loosely(t, err, should.ErrLike("token is for purpose BUILD"))

			_, err = parseToTokenBodyImpl(ctx, token, 123, pb.TokenBody_START_BUILD, pb.TokenBody_TASK)
			assert.Loosely(t, err, should.ErrLike("token is for purpose BUILD"))
		})

		t.Run("not base64 encoded token", func(t *ftt.Test) {
			_, err := parseToTokenBodyImpl(ctx, "invalid token", 123, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.ErrLike("error decoding token"))
		})

		t.Run("bad base64 encoded token", func(t *ftt.Test) {
			_, err := parseToTokenBodyImpl(ctx, "abckish", 123, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.ErrLike("error unmarshalling token"))
		})

		t.Run("unsupported token version", func(t *ftt.Test) {
			tkBody := &pb.TokenBody{
				BuildId: 1,
				State:   []byte("random"),
			}
			tkBytes, err := proto.Marshal(tkBody)
			assert.Loosely(t, err, should.BeNil)
			tkEnvelop := &pb.TokenEnvelope{
				Version: pb.TokenEnvelope_VERSION_UNSPECIFIED,
				Payload: tkBytes,
			}
			tkeBytes, err := proto.Marshal(tkEnvelop)
			assert.Loosely(t, err, should.BeNil)
			_, err = parseToTokenBodyImpl(ctx, base64.RawURLEncoding.EncodeToString(tkeBytes), 123, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.ErrLike("token with version 0 is not supported"))
		})
	})
}
