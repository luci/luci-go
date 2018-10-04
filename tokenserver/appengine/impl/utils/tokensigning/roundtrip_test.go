// Copyright 2017 The LUCI Authors.
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

package tokensigning

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	original := &messages.Subtoken{
		DelegatedIdentity: "user:delegated@example.com",
		RequestorIdentity: "user:requestor@example.com",
		CreationTime:      1477624966,
		ValidityDuration:  3600,
		Audience:          []string{"*"},
		Services:          []string{"*"},
	}
	signer := signingtest.NewSigner(&signing.ServiceInfo{
		ServiceAccountName: "service@example.com",
	})

	Convey("Sign/Inspect works (no signing context)", t, func() {
		tokSigner := signerForTest(signer, "")
		tokInspector := inspectorForTest(signer, "")

		tok, err := tokSigner.SignToken(ctx, original)
		So(err, ShouldBeNil)

		insp, err := tokInspector.InspectToken(ctx, tok)
		So(err, ShouldBeNil)

		So(insp.Signed, ShouldBeTrue)
		So(insp.Body, ShouldResembleProto, original)
	})

	Convey("Sign/Inspect works (with context)", t, func() {
		tokSigner := signerForTest(signer, "Some context")
		tokInspector := inspectorForTest(signer, "Some context")

		tok, err := tokSigner.SignToken(ctx, original)
		So(err, ShouldBeNil)

		insp, err := tokInspector.InspectToken(ctx, tok)
		So(err, ShouldBeNil)

		So(insp.Signed, ShouldBeTrue)
		So(insp.Body, ShouldResembleProto, original)
	})

	Convey("Sign/Inspect works (wrong context)", t, func() {
		tokSigner := signerForTest(signer, "Some context")
		tokInspector := inspectorForTest(signer, "Another context")

		tok, err := tokSigner.SignToken(ctx, original)
		So(err, ShouldBeNil)

		insp, err := tokInspector.InspectToken(ctx, tok)
		So(err, ShouldBeNil)

		So(insp.Signed, ShouldBeFalse)
	})
}
