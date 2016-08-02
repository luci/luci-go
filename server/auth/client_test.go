// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/auth"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRPCTransport(t *testing.T) {
	Convey("GetAccessToken works", t, func() {
		ctx := context.Background()
		mock := &clientRPCTransportMock{}
		ctx = ModifyConfig(ctx, func(cfg *Config) {
			cfg.AccessTokenProvider = mock.getAccessToken
			cfg.AnonymousTransport = mock.getTransport
		})

		Convey("in NoAuth mode", func(c C) {
			t, err := GetRPCTransport(ctx, NoAuth)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(&http.Request{})
			So(err, ShouldBeNil)

			So(len(mock.calls), ShouldEqual, 0)
			So(len(mock.reqs[0].Header), ShouldEqual, 0)
		})

		Convey("in AsSelf mode", func(c C) {
			t, err := GetRPCTransport(ctx, AsSelf, WithScopes("A", "B"))
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(&http.Request{})
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"A", "B"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer blah"},
			})
		})

		Convey("in AsSelf mode with default scopes", func(c C) {
			t, err := GetRPCTransport(ctx, AsSelf)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(&http.Request{})
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"https://www.googleapis.com/auth/userinfo.email"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer blah"},
			})
		})

		Convey("in NoAuth mode with scopes, should error", func(c C) {
			_, err := GetRPCTransport(ctx, NoAuth, WithScopes("A"))
			So(err, ShouldNotBeNil)
		})
	})

}

type clientRPCTransportMock struct {
	calls [][]string
	reqs  []*http.Request
}

func (m *clientRPCTransportMock) getAccessToken(c context.Context, scopes []string) (auth.Token, error) {
	m.calls = append(m.calls, scopes)
	return auth.Token{AccessToken: "blah", TokenType: "Bearer"}, nil
}

func (m *clientRPCTransportMock) getTransport(c context.Context) http.RoundTripper {
	return m
}

func (m *clientRPCTransportMock) RoundTrip(req *http.Request) (*http.Response, error) {
	m.reqs = append(m.reqs, req)
	return &http.Response{}, nil
}
