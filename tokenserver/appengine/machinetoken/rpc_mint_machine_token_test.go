// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package machinetoken

import (
	"crypto/x509"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/proto/google"

	minter "github.com/luci/luci-go/tokenserver/api/minter/v1"
	"github.com/luci/luci-go/tokenserver/appengine/certconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintMachineTokenRPC(t *testing.T) {
	Convey("works", t, func() {
		ctx := testingContext()

		impl := MintMachineTokenRPC{
			Signer: testingSigner(),
			CheckCertificate: func(_ context.Context, cert *x509.Certificate) (*certconfig.CA, error) {
				return &testingCA, nil
			},
		}

		resp, err := impl.MintMachineToken(ctx, testingMachineTokenRequest(ctx))
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &minter.MintMachineTokenResponse{
			ServiceVersion: "unit-tests/mocked-ver",
			TokenResponse: &minter.MachineTokenResponse{
				ServiceVersion: "unit-tests/mocked-ver",
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: expectedLuciMachineToken,
						Expiry:       google.NewTimestamp(clock.Now(ctx).Add(time.Hour)),
					},
				},
			},
		})
	})
}
