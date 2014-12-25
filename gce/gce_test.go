// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gce

import (
	"net"
	"net/http"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// resetIsGCECached clear process wide cache of IsRunningOnGCE().
func resetIsGCECached() {
	isGCELock.Lock()
	isGCECached = nil
	isGCELock.Unlock()
}

func mockIsRunningOnGCE() {
	val := true
	isGCELock.Lock()
	isGCECached = &val
	isGCELock.Unlock()
}

func TestGCE(t *testing.T) {
	// TODO: This test is not concurrency safe, it mocks global gceMetadataServer.

	// HTTP status to return from mocked GCE server.
	mockedDataLock := sync.Mutex{}
	mockedPath := ""
	mockedStatus := 404
	mockedBody := ""

	// mockResponse sets how to reply on the next request to mocked metadata server.
	mockResponse := func(path string, status int, body string) {
		mockedDataLock.Lock()
		defer mockedDataLock.Unlock()
		mockedPath = "/computeMetadata/v1" + path
		mockedStatus = status
		mockedBody = body
	}

	// Launch a server that mocks GCE metadata server.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		mockedDataLock.Lock()
		defer mockedDataLock.Unlock()
		if req.RequestURI != mockedPath {
			t.Errorf("Expecting request to %s, got %s", mockedPath, req.RequestURI)
		}
		resp.WriteHeader(mockedStatus)
		if mockedBody != "" {
			resp.Write([]byte(mockedBody))
		}
	})

	// Pick a random port, start listenting.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.FailNow()
	}
	tcpListener := ln.(*net.TCPListener)
	mockedAddress := tcpListener.Addr()

	// Launch a server there, kill it when test case ends.
	server := &http.Server{Handler: mux}
	go func() { server.Serve(tcpListener) }()
	defer tcpListener.Close()

	Convey("Given mocked server", t, func() {
		resetIsGCECached()
		mockResponse("/", 404, "")

		prevMetadataServer := gceMetadataServer
		gceMetadataServer = "http://" + mockedAddress.String()

		Reset(func() {
			gceMetadataServer = prevMetadataServer
		})

		Convey("Not GCE", func() {
			So(IsRunningOnGCE(), ShouldBeFalse)
		})

		Convey("On GCE", func() {
			mockResponse("/", 200, "")
			So(IsRunningOnGCE(), ShouldBeTrue)
		})

		Convey("Not a valid host", func() {
			gceMetadataServer = "http://not_a_host"
			So(IsRunningOnGCE(), ShouldBeFalse)
		})

		Convey("Check process cache", func() {
			// Exercise code path that touches process cache.
			So(IsRunningOnGCE(), ShouldBeFalse)
			So(IsRunningOnGCE(), ShouldBeFalse)
		})

		Convey("Test GetAccessToken", func() {
			mockIsRunningOnGCE()
			mockResponse(
				"/instance/service-accounts/default/token", 200,
				`{"access_token": "123", "expires_in": 11111}`)
			tok, err := GetAccessToken("default")
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &AccessToken{AccessToken: "123", ExpiresIn: 11111})
		})

		Convey("Test GetServiceAccount", func() {
			mockIsRunningOnGCE()
			mockResponse(
				"/instance/service-accounts/default/?recursive=true", 200,
				`{"email": "email@example.com", "scopes": ["scope1", "scope2"]}`)
			acc, err := GetServiceAccount("default")
			So(err, ShouldBeNil)
			So(acc, ShouldResemble, &ServiceAccount{
				Email:  "email@example.com",
				Scopes: []string{"scope1", "scope2"},
			})
		})

		Convey("Test GetInstanceAttribute present", func() {
			mockIsRunningOnGCE()
			mockResponse("/instance/attributes/key", 200, "value")
			attr, err := GetInstanceAttribute("key")
			So(err, ShouldBeNil)
			So(attr, ShouldResemble, []byte("value"))
		})

		Convey("Test GetInstanceAttribute missing", func() {
			mockIsRunningOnGCE()
			mockResponse("/instance/attributes/key", 404, "???")
			attr, err := GetInstanceAttribute("key")
			So(err, ShouldBeNil)
			So(attr, ShouldBeNil)
		})
	})
}
