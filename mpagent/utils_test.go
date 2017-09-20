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

// Contains tests for utility functions.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"golang.org/x/net/context"
)

func TestAck(t *testing.T) {
	t.Parallel()

	convey.Convey("ack works", t, func(c convey.C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), convey.ShouldEqual, "application/json")
			c.So(r.Method, convey.ShouldEqual, "POST")

			w.WriteHeader(204)
		}))
		defer ts.Close()

		e := ack(context.Background(), &http.Client{}, ts.URL, []byte{})
		convey.So(e, convey.ShouldBeNil)
	})

	convey.Convey("Returns 404", t, func(c convey.C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), convey.ShouldEqual, "application/json")
			c.So(r.Method, convey.ShouldEqual, "POST")

			w.WriteHeader(404)
		}))
		defer ts.Close()

		e := ack(context.Background(), &http.Client{}, ts.URL, []byte{})
		convey.So(e, convey.ShouldNotBeNil)
		convey.So(e.Error(), convey.ShouldEqual, "Unexpected HTTP status: 404 Not Found.")
	})
}

func TestPoll(t *testing.T) {
	t.Parallel()

	convey.Convey("poll works", t, func(c convey.C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), convey.ShouldEqual, "application/json")
			c.So(r.Method, convey.ShouldEqual, "POST")

			response := struct {
				Instruction struct {
					SwarmingServer string	`json:"swarming_server"`
				}		`json:"instruction"`
				State string	`json:"state"`
			}{
			}

			w.WriteHeader(200)
			json.NewEncoder(w).Encode(response)
		}))
		defer ts.Close()

		i, e := poll(context.Background(), &http.Client{}, ts.URL, []byte{})
		convey.So(i.State, convey.ShouldEqual, "")
		convey.So(e, convey.ShouldBeNil)
	})

	convey.Convey("Returns 404", t, func(c convey.C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Header.Get("Content-Type"), convey.ShouldEqual, "application/json")
			c.So(r.Method, convey.ShouldEqual, "POST")

			w.WriteHeader(404)
		}))
		defer ts.Close()

		i, e := poll(context.Background(), &http.Client{}, ts.URL, []byte{})
		convey.So(i, convey.ShouldBeNil)
		convey.So(e.Error(), convey.ShouldEqual, "Unexpected HTTP status: 404 Not Found.")
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

	convey.Convey("substitute works", t, func() {
		s, e := substitute(context.Background(), "Test {{.Field1}} {{.Field2}} {{.Field3}}", substitutions)
		convey.So(s, convey.ShouldEqual, "Test Value 1 Value 2 Value 3")
		convey.So(e, convey.ShouldBeNil)
	})

	convey.Convey("Not all substitutions present in template", t, func() {
		s, e := substitute(context.Background(), "Test {{.Field2}}", substitutions)
		convey.So(s, convey.ShouldEqual, "Test Value 2")
		convey.So(e, convey.ShouldBeNil)
	})

	convey.Convey("Template parsing error", t, func() {
		s, e := substitute(context.Background(), "Test {{", substitutions)
		convey.So(s, convey.ShouldEqual, "")
		convey.So(e, convey.ShouldNotBeNil)
	})
}
