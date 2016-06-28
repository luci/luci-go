// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package signing

import (
	"errors"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/router"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestContext(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()

		So(GetSigner(ctx), ShouldBeNil)
		_, _, err := SignBytes(ctx, nil)
		So(err, ShouldErrLike, "no Signer in the context")

		ctx = SetSigner(ctx, phonySigner{})
		_, _, err = SignBytes(ctx, nil)
		So(err, ShouldBeNil)
	})
}

func TestHandlers(t *testing.T) {
	call := func(s Signer) (*PublicCertificates, error) {
		r := router.New()
		InstallHandlers(r, router.NewMiddlewareChain(
			func(c *router.Context, next router.Handler) {
				c.Context = SetSigner(context.Background(), s)
				next(c)
			},
		))
		ts := httptest.NewServer(r)
		return FetchCertificates(context.Background(), ts.URL+"/auth/api/v1/server/certificates")
	}

	Convey("Works", t, func() {
		certs, err := call(phonySigner{})
		So(err, ShouldBeNil)
		So(len(certs.Certificates), ShouldEqual, 1)
	})

	Convey("No signer", t, func() {
		_, err := call(nil)
		So(err, ShouldErrLike, "HTTP code (404)")
	})

	Convey("Error getting certs", t, func() {
		_, err := call(phonySigner{errors.New("fail")})
		So(err, ShouldErrLike, "HTTP code (500)")
	})
}

///

type phonySigner struct {
	certErr error
}

func (s phonySigner) SignBytes(c context.Context, blob []byte) (string, []byte, error) {
	return "phonyKey", []byte("signature"), nil
}

func (s phonySigner) Certificates(c context.Context) (*PublicCertificates, error) {
	if s.certErr != nil {
		return nil, s.certErr
	}
	return &PublicCertificates{
		Certificates: []Certificate{
			{
				KeyName:            "phonyKey",
				X509CertificatePEM: "phonyPEM",
			},
		},
	}, nil
}
