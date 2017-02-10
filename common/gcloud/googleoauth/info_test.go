// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package googleoauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetTokenInfo(t *testing.T) {
	Convey("Works", t, func() {
		goodToken := true
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				panic("not a GET")
			}
			if r.RequestURI != "/?access_token=zzz%3D%3D%3Dtoken" {
				panic("wrong URI " + r.RequestURI)
			}
			if goodToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(`{"aud":"blah", "expires_in":"12345", "email_verified": "true"}`))
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				w.Write([]byte(`{"error_description":"Invalid value"}`))
			}
		}))
		defer ts.Close()

		info, err := GetTokenInfo(context.Background(), TokenInfoParams{
			AccessToken: "zzz===token",
			Endpoint:    ts.URL,
		})
		So(err, ShouldBeNil)
		So(info, ShouldResemble, &TokenInfo{
			Aud:           "blah",
			ExpiresIn:     12345,
			EmailVerified: true,
		})

		goodToken = false
		info, err = GetTokenInfo(context.Background(), TokenInfoParams{
			AccessToken: "zzz===token",
			Endpoint:    ts.URL,
		})
		So(info, ShouldBeNil)
		So(err, ShouldEqual, ErrBadToken)
	})
}
