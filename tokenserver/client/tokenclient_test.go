// Copyright 2016 The LUCI Authors.
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

package client

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/google"

	tokenserver "github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenClient(t *testing.T) {
	Convey("works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 7, time.UTC))

		expectedResp := &minter.MachineTokenResponse{
			TokenType: &minter.MachineTokenResponse_LuciMachineToken{
				LuciMachineToken: &minter.LuciMachineToken{
					MachineToken: "blah",
				},
			},
		}

		c := Client{
			Client: &fakeRPCClient{
				Out: minter.MintMachineTokenResponse{TokenResponse: expectedResp},
			},
			Signer: &fakeSigner{},
		}

		resp, err := c.MintMachineToken(ctx, &minter.MachineTokenRequest{
			TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, expectedResp)

		rpc := c.Client.(*fakeRPCClient).In
		So(rpc.Signature, ShouldResemble, []byte("fake signature"))

		tokReq := minter.MachineTokenRequest{}
		So(proto.Unmarshal(rpc.SerializedTokenRequest, &tokReq), ShouldBeNil)
		So(tokReq, ShouldResemble, minter.MachineTokenRequest{
			Certificate:        []byte("fake certificate"),
			SignatureAlgorithm: minter.SignatureAlgorithm_SHA256_RSA_ALGO,
			IssuedAt:           google.NewTimestamp(clock.Now(ctx)),
			TokenType:          tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
		})
	})

	Convey("handles error", t, func() {
		ctx := context.Background()

		c := Client{
			Client: &fakeRPCClient{
				Out: minter.MintMachineTokenResponse{
					ErrorCode:    1234,
					ErrorMessage: "blah",
				},
			},
			Signer: &fakeSigner{},
		}

		_, err := c.MintMachineToken(ctx, &minter.MachineTokenRequest{
			TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
		})
		So(err.Error(), ShouldEqual, "token server error 1234 - blah")
	})
}

// fakeRPCClient implements minter.TokenMinterClient.
type fakeRPCClient struct {
	In  minter.MintMachineTokenRequest
	Out minter.MintMachineTokenResponse
}

func (f *fakeRPCClient) MintMachineToken(ctx context.Context, in *minter.MintMachineTokenRequest, opts ...grpc.CallOption) (*minter.MintMachineTokenResponse, error) {
	f.In = *in
	return &f.Out, nil
}

// fakeSigner implements Signer.
type fakeSigner struct{}

func (f *fakeSigner) Algo(ctx context.Context) (x509.SignatureAlgorithm, error) {
	return x509.SHA256WithRSA, nil
}

func (f *fakeSigner) Certificate(ctx context.Context) ([]byte, error) {
	return []byte("fake certificate"), nil
}

func (f *fakeSigner) Sign(ctx context.Context, blob []byte) ([]byte, error) {
	return []byte("fake signature"), nil
}
