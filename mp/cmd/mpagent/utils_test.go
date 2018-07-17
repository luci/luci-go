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
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAck(t *testing.T) {
	t.Parallel()

	Convey("ack works", t, func(c C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")
			c.So(r.Method, ShouldEqual, "POST")

			j := struct {
				Backend  string `json:"backend"`
				Hostname string `json:"hostname"`
			}{}
			b, e := ioutil.ReadAll(r.Body)
			c.So(e, ShouldBeNil)
			e = json.Unmarshal(b, &j)
			c.So(e, ShouldBeNil)
			c.So(j.Backend, ShouldEqual, "backend")
			c.So(j.Hostname, ShouldEqual, "hostname")

			w.WriteHeader(204)
		}))
		defer ts.Close()

		ctx := context.Background()
		mp, e := getClient(ctx, &http.Client{}, ts.URL)
		So(e, ShouldBeNil)

		e = mp.ack(ctx, "hostname", "backend")
		So(e, ShouldBeNil)
	})

	Convey("Returns 404", t, func(c C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")
			c.So(r.Method, ShouldEqual, "POST")

			j := struct {
				Backend  string `json:"backend"`
				Hostname string `json:"hostname"`
			}{}
			b, e := ioutil.ReadAll(r.Body)
			c.So(e, ShouldBeNil)
			e = json.Unmarshal(b, &j)
			c.So(e, ShouldBeNil)
			c.So(j.Backend, ShouldEqual, "backend")
			c.So(j.Hostname, ShouldEqual, "hostname")

			w.WriteHeader(404)
		}))
		defer ts.Close()

		ctx := context.Background()
		mp, e := getClient(ctx, &http.Client{}, ts.URL)
		So(e, ShouldBeNil)

		e = mp.ack(context.Background(), "hostname", "backend")
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "404")
	})
}

func TestPoll(t *testing.T) {
	t.Parallel()

	Convey("poll works", t, func(c C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")
			c.So(r.Method, ShouldEqual, "POST")

			j := struct {
				Backend  string `json:"backend"`
				Hostname string `json:"hostname"`
			}{}
			b, e := ioutil.ReadAll(r.Body)
			c.So(e, ShouldBeNil)
			e = json.Unmarshal(b, &j)
			c.So(e, ShouldBeNil)
			c.So(j.Backend, ShouldEqual, "backend")
			c.So(j.Hostname, ShouldEqual, "hostname")

			response := struct {
				Instruction struct {
					SwarmingServer string `json:"swarming_server"`
				} `json:"instruction"`
				State string `json:"state"`
			}{}

			w.WriteHeader(200)
			json.NewEncoder(w).Encode(response)
		}))
		defer ts.Close()

		ctx := context.Background()
		mp, e := getClient(ctx, &http.Client{}, ts.URL)
		So(e, ShouldBeNil)

		i, e := mp.poll(ctx, "hostname", "backend")
		So(i.State, ShouldEqual, "")
		So(e, ShouldBeNil)
	})

	Convey("Returns 404", t, func(c C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")
			c.So(r.Method, ShouldEqual, "POST")

			j := struct {
				Backend  string `json:"backend"`
				Hostname string `json:"hostname"`
			}{}
			b, e := ioutil.ReadAll(r.Body)
			c.So(e, ShouldBeNil)
			e = json.Unmarshal(b, &j)
			c.So(e, ShouldBeNil)
			c.So(j.Backend, ShouldEqual, "backend")
			c.So(j.Hostname, ShouldEqual, "hostname")

			w.WriteHeader(404)
		}))
		defer ts.Close()

		ctx := context.Background()
		mp, e := getClient(ctx, &http.Client{}, ts.URL)
		So(e, ShouldBeNil)

		i, e := mp.poll(ctx, "hostname", "backend")
		So(i, ShouldBeNil)
		So(e, ShouldErrLike, "404")
	})
}

func TestSubstitute(t *testing.T) {
	t.Parallel()

	substitutions := struct {
		Field1 string
		Field2 string
		Field3 string
	}{
		Field1: "Value 1",
		Field2: "Value 2",
		Field3: "Value 3",
	}

	Convey("substitute works", t, func() {
		s, e := substitute(context.Background(), "Test {{.Field1}} {{.Field2}} {{.Field3}}", substitutions)
		So(s, ShouldEqual, "Test Value 1 Value 2 Value 3")
		So(e, ShouldBeNil)
	})

	Convey("Not all substitutions present in template", t, func() {
		s, e := substitute(context.Background(), "Test {{.Field2}}", substitutions)
		So(s, ShouldEqual, "Test Value 2")
		So(e, ShouldBeNil)
	})

	Convey("Template parsing error", t, func() {
		s, e := substitute(context.Background(), "Test {{", substitutions)
		So(s, ShouldEqual, "")
		So(e, ShouldNotBeNil)
	})
}
