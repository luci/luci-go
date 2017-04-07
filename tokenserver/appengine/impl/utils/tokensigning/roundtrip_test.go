// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tokensigning

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
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
	signer := signingtest.NewSigner(0, &signing.ServiceInfo{
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
		So(insp.Body, ShouldResemble, original)
	})

	Convey("Sign/Inspect works (with context)", t, func() {
		tokSigner := signerForTest(signer, "Some context")
		tokInspector := inspectorForTest(signer, "Some context")

		tok, err := tokSigner.SignToken(ctx, original)
		So(err, ShouldBeNil)

		insp, err := tokInspector.InspectToken(ctx, tok)
		So(err, ShouldBeNil)

		So(insp.Signed, ShouldBeTrue)
		So(insp.Body, ShouldResemble, original)
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
