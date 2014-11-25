// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gce

import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsRunningOnGCE(t *testing.T) {
	// TODO: This test is not concurrency safe, it mocks global
	// gceMetadataServer and gceUtilTempDir.

	// HTTP, "Metadata-Flavor" header to return from mocked GCE server.
	mockedDataLock := sync.Mutex{}
	mockedStatus := 404
	mockedFlavor := "Unknown"

	// mockResponse sets how to reply on the next request to mocked metadata server.
	mockResponse := func(status int, flavor string) {
		mockedDataLock.Lock()
		defer mockedDataLock.Unlock()
		mockedStatus = status
		mockedFlavor = flavor
	}

	// Launch a server that mocks GCE metadata server.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		mockedDataLock.Lock()
		defer mockedDataLock.Unlock()
		resp.Header()["Metadata-Flavor"] = []string{mockedFlavor}
		resp.WriteHeader(mockedStatus)
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

	Convey("Given a temp directory and mocked server", t, func() {
		resetIsGCECached()
		mockResponse(404, "Unknown")

		prevTempDir := gceUtilTempDir
		gceUtilTempDir, _ = ioutil.TempDir("", "go_infra_utils_test")

		prevMetadataServer := gceMetadataServer
		gceMetadataServer = "http://" + mockedAddress.String()

		Reset(func() {
			os.RemoveAll(gceUtilTempDir)
			gceUtilTempDir = prevTempDir
			gceMetadataServer = prevMetadataServer
		})

		Convey("Not GCE", func() {
			So(IsRunningOnGCE(), ShouldBeFalse)
		})

		Convey("On GCE", func() {
			mockResponse(200, "Google")
			So(IsRunningOnGCE(), ShouldBeTrue)
		})

		Convey("Not google", func() {
			mockResponse(200, "Huh?")
			So(IsRunningOnGCE(), ShouldBeFalse)
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

		Convey("Check file cache", func() {
			// Exercise code path that touches cache file on disk.
			So(IsRunningOnGCE(), ShouldBeFalse)
			resetIsGCECached()
			So(IsRunningOnGCE(), ShouldBeFalse)
		})
	})
}

func TestReadWriteBool(t *testing.T) {
	Convey("Given a temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "go_infra_utils_test")
		So(err, ShouldBeNil)
		Reset(func() { os.RemoveAll(tempDir) })

		Convey("Write then read works", func() {
			for _, value := range []bool{true, false} {
				err := writeBoolToFile(filepath.Join(tempDir, "abc"), value)
				So(err, ShouldBeNil)
				read, err := readBoolFromFile(filepath.Join(tempDir, "abc"))
				So(read, ShouldEqual, value)
				So(err, ShouldBeNil)
			}
		})

		Convey("Read missing", func() {
			_, err := readBoolFromFile(filepath.Join(tempDir, "abc"))
			So(err, ShouldNotBeNil)
		})
	})
}
