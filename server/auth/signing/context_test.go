// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signing

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/middleware"

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
		r := httprouter.New()
		InstallHandlers(r, func(h middleware.Handler) httprouter.Handle {
			return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
				ctx := SetSigner(context.Background(), s)
				h(ctx, w, r, p)
			}
		})
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
