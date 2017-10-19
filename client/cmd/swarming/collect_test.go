// Copyright 2017 The LUCI Authors.
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

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)


func TestCollectParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.Parse(&[]string{})
		So(err, ShouldErrLike, "must provide -server")
	})
}


func TestCollectParse_NoInput(t *testing.T) {
	Convey(`Make sure that Parse handles no task IDs given.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse(&[]string{})
		So(err, ShouldErrLike, "must specify at least one")
	})
}

func TestCollectParse_BadTaskID(t *testing.T) {
	Convey(`Make sure that Parse handles a malformed task ID.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse(&[]string{"$$$$$"})
		So(err, ShouldErrLike, "task ID")
	})
}

func TestCollectParse_BadTimeout(t *testing.T) {
	Convey(`Make sure that Parse handles a negative timeout.`, t, func() {
		c := collectRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-timeout", "-30m",
		})

		err = c.Parse(&[]string{"x81n8xn1b684n"})
		So(err, ShouldErrLike, "negative timeout")
	})
}

func testCollectPollWithServer(tout string, handler func(http.ResponseWriter, *http.Request)) taskResult {
	// Set up test server.
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	// Set up test swarming service.
	s, err := swarming.New(&http.Client{})
	So(err, ShouldBeNil)
	s.BasePath = ts.URL

	// Set up context.
	timeout, err := time.ParseDuration(tout)
	So(err, ShouldBeNil)
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	results := make(chan taskResult, 1)
	runner := collectRun{}
	go runner.pollForTaskResult(ctx, "10982374012938470", s, results)
	ret := <-results
	return ret
}

func TestCollectPollForTaskResult(t *testing.T) {
	t.Parallel()

	Convey(`Test fatal response`, t, func() {
		result := testCollectPollWithServer("1s", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		})
		So(result.Error, ShouldErrLike, "404")
	})

	Convey(`Test timeout exceeded`, t, func() {
		result := testCollectPollWithServer("100ms", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(502)
		})
		So(result.Error, ShouldErrLike, "context deadline exceeded")
	})

	Convey(`Test bot finished`, t, func(c C) {
		result := testCollectPollWithServer("1s", func(w http.ResponseWriter, r *http.Request) {
			err := json.NewEncoder(w).Encode(&swarming.SwarmingRpcsTaskResult{State: "COMPLETED"})
			c.So(err, ShouldBeNil)
		})
		So(result.Error, ShouldBeNil)
		So(result.Result, ShouldNotBeNil)
		So(result.Result.State, ShouldResemble, "COMPLETED")
	})
}
