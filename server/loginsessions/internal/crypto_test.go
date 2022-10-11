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

package internal

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRandom(t *testing.T) {
	t.Parallel()

	Convey("RandomBlob", t, func() {
		So(RandomBlob(40), ShouldHaveLength, 40)
	})

	Convey("RandomAlphaNum", t, func() {
		So(RandomAlphaNum(40), ShouldHaveLength, 40)
	})
}

func TestState(t *testing.T) {
	t.Parallel()

	Convey("Round trip", t, func() {
		ctx := secrets.GeneratePrimaryTinkAEADForTest(context.Background())

		original := &statepb.OpenIDState{
			LoginSessionId:   "login-session-id",
			LoginCookieValue: "cookie-value",
		}

		blob, err := EncryptState(ctx, original)
		So(err, ShouldBeNil)

		decrypted, err := DecryptState(ctx, blob)
		So(err, ShouldBeNil)

		So(decrypted, ShouldResembleProto, original)
	})

}
