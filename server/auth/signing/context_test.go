// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package signing

import (
	"errors"
	"net/http"
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
		ctx = SetSigner(ctx, &phonySigner{})
		So(GetSigner(ctx), ShouldNotBeNil)
	})
}

func TestCertificatesHandler(t *testing.T) {
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
		certs, err := call(&phonySigner{})
		So(err, ShouldBeNil)
		So(len(certs.Certificates), ShouldEqual, 1)
	})

	Convey("No signer", t, func() {
		_, err := call(nil)
		So(err, ShouldErrLike, "HTTP code (404)")
	})

	Convey("Error getting certs", t, func() {
		_, err := call(&phonySigner{errors.New("fail")})
		So(err, ShouldErrLike, "HTTP code (500)")
	})
}

func TestServiceInfoHandler(t *testing.T) {
	Convey("Works", t, func() {
		r := router.New()
		signer := &phonySigner{}

		InstallHandlers(r, router.NewMiddlewareChain(
			func(ctx *router.Context, next router.Handler) {
				ctx.Context = SetSigner(context.Background(), signer)
				next(ctx)
			},
		))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/api/v1/server/info", nil)
		r.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 200)
		So(w.Body.String(), ShouldResemble,
			`{"app_id":"phony-app","app_runtime":"go",`+
				`"app_runtime_version":"go1.5.1",`+
				`"app_version":"1234-abcdef","service_account_name":`+
				`"phony-app-account@example.com"}`+"\n")

		signer.err = errors.New("fail")

		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/auth/api/v1/server/info", nil)
		r.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 500)
		So(w.Body.String(), ShouldResemble, "{\"error\":\"Can't grab service info - fail\"}\n")
	})
}

///

type phonySigner struct {
	err error
}

func (s *phonySigner) SignBytes(c context.Context, blob []byte) (string, []byte, error) {
	if s.err != nil {
		return "", nil, s.err
	}
	return "phonyKey", []byte("signature"), nil
}

func (s *phonySigner) Certificates(c context.Context) (*PublicCertificates, error) {
	if s.err != nil {
		return nil, s.err
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

func (s *phonySigner) ServiceInfo(c context.Context) (*ServiceInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &ServiceInfo{
		AppID:              "phony-app",
		AppRuntime:         "go",
		AppRuntimeVersion:  "go1.5.1",
		AppVersion:         "1234-abcdef",
		ServiceAccountName: "phony-app-account@example.com",
	}, nil
}
