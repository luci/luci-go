// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGroupsService(t *testing.T) {
	Convey("Mocking time and HTTP", t, func() {
		mockSleep()

		requests := mockDoRequest()
		service := NewGroupsService("https://example.com", &http.Client{}, nil)

		Convey("doGet works", func() {
			requests.expect("https://example.com/some/path", 200, `{"key":"val"}`)
			var response struct {
				Key string `json:"key"`
			}
			err := service.doGet("/some/path", &response)
			So(err, ShouldBeNil)
			So(response.Key, ShouldEqual, "val")
		})

		Convey("doGet retry works", func() {
			requests.expect("https://example.com/some/path", 500, "")
			requests.expect("https://example.com/some/path", 503, "")
			requests.expect("https://example.com/some/path", 200, `{"key":"val"}`)
			var response struct {
				Key string `json:"key"`
			}
			err := service.doGet("/some/path", &response)
			So(err, ShouldBeNil)
			So(response.Key, ShouldEqual, "val")
		})

		Convey("doGet fatal error", func() {
			requests.expect("https://example.com/some/path", 403, "error")
			var response struct{}
			err := service.doGet("/some/path", &response)
			So(err, ShouldNotBeNil)
		})

		Convey("doGet max retry", func() {
			for i := 0; i < 5; i++ {
				requests.expect("https://example.com/some/path", 500, "")
			}
			var response struct{}
			err := service.doGet("/some/path", &response)
			So(err, ShouldNotBeNil)
		})

		Convey("FetchCallerIdentity works", func() {
			requests.expect("https://example.com/auth/api/v1/accounts/self", 200, `{
				"identity": "user:abc@example.com"
			}`)
			ident, err := service.FetchCallerIdentity()
			So(err, ShouldBeNil)
			So(ident, ShouldResemble, Identity{
				Kind: IdentityKindUser,
				Name: "abc@example.com",
			})
		})

		Convey("FetchGroup works", func() {
			requests.expect("https://example.com/auth/api/v1/groups/abc/name", 200, `{
				"group": {
					"name": "abc/name",
					"description": "Some description",
					"members": [
						"anonymous:anonymous",
						"bot:127.0.0.1",
						"service:some-gae-app",
						"user:abc@example.com",
						"broken id",
						"unknown_type:abc"
					],
					"globs": ["user:*@example.com"],
					"nested": ["another-group"]
				}
			}`)
			group, err := service.FetchGroup("abc/name")
			So(err, ShouldBeNil)
			So(group, ShouldResemble, Group{
				Name:        "abc/name",
				Description: "Some description",
				Members: []Identity{
					Identity{Kind: IdentityKindAnonymous, Name: "anonymous"},
					Identity{Kind: IdentityKindBot, Name: "127.0.0.1"},
					Identity{Kind: IdentityKindService, Name: "some-gae-app"},
					Identity{Kind: IdentityKindUser, Name: "abc@example.com"},
				},
				Globs:  []string{"user:*@example.com"},
				Nested: []string{"another-group"},
			})
		})
	})
}

func mockSleep() {
	prev := sleep
	sleep = func(d time.Duration) {}
	Reset(func() {
		sleep = prev
	})
}

type mockedRequest struct {
	URL        string
	StatusCode int
	Response   string
}

type mockedRequests struct {
	Expected []mockedRequest
}

func mockDoRequest() *mockedRequests {
	requests := &mockedRequests{}
	prev := doRequest
	doRequest = func(c *http.Client, req *http.Request) (*http.Response, error) {
		So(len(requests.Expected), ShouldNotEqual, 0)
		next := requests.Expected[0]
		requests.Expected = requests.Expected[1:]
		So(next.URL, ShouldEqual, req.URL.String())
		return &http.Response{
			StatusCode: next.StatusCode,
			Body:       ioutil.NopCloser(strings.NewReader(next.Response)),
		}, nil
	}
	Reset(func() {
		requests.assertExecuted()
		doRequest = prev
	})
	return requests
}

func (r *mockedRequests) expect(url string, status int, response string) {
	r.Expected = append(r.Expected, mockedRequest{
		URL:        url,
		StatusCode: status,
		Response:   response,
	})
}

func (r *mockedRequests) assertExecuted() {
	So(r.Expected, ShouldBeEmpty)
}
