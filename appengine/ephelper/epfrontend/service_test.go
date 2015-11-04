// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type serviceBackendStub struct {
	body     bytes.Buffer
	callback func(w http.ResponseWriter)
}

func (s *serviceBackendStub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	_, err := s.body.ReadFrom(req.Body)
	if err != nil {
		panic(err)
	}

	if s.callback != nil {
		s.callback(w)
	}
}

type requestNormalizer struct {
	http.Handler
	scheme string
	host   string
}

func (s *requestNormalizer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	u, err := url.ParseRequestURI(req.RequestURI)
	if err != nil {
		panic(err)
	}

	u.Scheme = s.scheme
	u.Host = s.host
	req.URL = u
	req.RequestURI = u.Path
	req.Host = u.Host
	s.Handler.ServeHTTP(w, req)
}

type ServiceTestService struct {
}

type ServiceTestPathReq struct {
	Name  string `endpoints:"required"`
	Count int64  `endpoints:"required"`
}

type ServiceTestResponse struct {
	Count int64 `json:"count"`
}

func (s *ServiceTestService) PathReq(c context.Context, req *ServiceTestPathReq) (*ServiceTestResponse, error) {
	return nil, endpoints.UnauthorizedError
}

type ServiceTestQueryReq struct {
	Name  string `endpoints:"required"`
	Count int64  `json:"count"`

	S  string
	F  float32
	B  bool
	B2 bool
	I  int32
}

func (s *ServiceTestService) QueryReq(c context.Context, req *ServiceTestQueryReq) error {
	return endpoints.UnauthorizedError
}

type ServiceTestPostReq struct {
	S string
	T string
}

func (s *ServiceTestService) Post(c context.Context, req *ServiceTestPostReq) error {
	return endpoints.UnauthorizedError
}

func readErrorResponse(resp *http.Response) *outerError {
	defer resp.Body.Close()

	e := outerError{}
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return nil
	}
	return &e
}

func TestService(t *testing.T) {
	Convey(`A testing service`, t, func() {
		be := serviceBackendStub{}
		fe := New("", &be)

		ts := httptest.NewServer(&requestNormalizer{
			Handler: fe,
			scheme:  "http",
			host:    "example.com",
		})
		defer ts.Close()

		c := http.Client{}

		Convey(`Requests to non-root URLs are rejected.`, func() {
			resp, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/ohai"))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)
		})

		Convey(`With a registered test service (query parameters)`, func() {
			// Create an endpoints backend because we need the API descriptors that it
			// creates from our services.
			epbe := endpoints.NewServer("")
			svc, err := epbe.RegisterService(&ServiceTestService{}, "test", "v1", "Test Service", true)
			So(err, ShouldBeNil)
			So(svc, ShouldNotBeNil)

			m := svc.MethodByName("QueryReq")
			m.Info().HTTPMethod = "GET"
			m.Info().Path = "queryreq/{Name}"

			So(fe.RegisterService(svc), ShouldBeNil)

			Convey(`Will not register the service again.`, func() {
				So(fe.RegisterService(svc), ShouldNotBeNil)
			})

			Convey(`Exposes a redirecting "explorer" API.`, func() {
				redirected := false
				c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
					redirected = true
					return errors.New("testing, no redirect")
				}

				c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/explorer"))
				So(redirected, ShouldBeTrue)
			})

			Convey(`POST requests to directory endpoint return http.StatusMethodNotAllowed.`, func() {
				resp, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/discovery/v1/apis"), "", nil)
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusMethodNotAllowed)
			})

			Convey(`Exposes a directory REST endpoint.`, func() {
				exp := directoryList{}
				loadJSONTestCase(&exp, "service", "test", "directory")

				act := directoryList{}
				resp, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/discovery/v1/apis"))
				So(err, ShouldBeNil)
				defer resp.Body.Close()
				So(json.NewDecoder(resp.Body).Decode(&act), ShouldBeNil)

				So(act, assertions.ShouldResembleV, exp)
			})

			Convey(`Exposes a service REST endpoint.`, func() {
				exp := restDescription{}
				loadJSONTestCase(&exp, "service", "test", "service")

				act := restDescription{}
				resp, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/discovery/v1/apis/test/v1/rest"))
				So(err, ShouldBeNil)
				defer resp.Body.Close()
				So(json.NewDecoder(resp.Body).Decode(&act), ShouldBeNil)

				So(act, assertions.ShouldResembleV, exp)
			})

			Convey(`Can access the "pathreq" endpoint with path elements.`, func() {
				_, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/pathreq/testname/12345"))
				So(err, ShouldBeNil)

				So(be.body.String(), ShouldEqual, `{"Count":12345,"Name":"testname"}`+"\n")
			})

			Convey(`Will return an error if an invalid "pathreq" path parameter is supplied.`, func() {
				resp, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/pathreq/testname/pi"))
				So(err, ShouldBeNil)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusBadRequest)
				So(e.Error.Message, ShouldEqual, "unspecified error")
			})

			Convey(`Can access the "queryreq" endpoint with query parameters.`, func() {
				_, err := c.Get(fmt.Sprintf("%s%s", ts.URL,
					"/_ah/api/test/v1/queryreq/testname?count=12345&S=foo&F=3.14&B=true&B2=false&I=1337"))
				So(err, ShouldBeNil)

				So(be.body.String(), ShouldEqual,
					`{"B":true,"B2":false,"F":3.14,"I":1337,"Name":"testname","S":"foo","count":12345}`+"\n")
			})

			Convey(`Will return an error if an invalid "queryreq" query parameter is supplied.`, func() {
				resp, err := c.Get(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/queryreq/testname?count=pi"))
				So(err, ShouldBeNil)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusBadRequest)
				So(e.Error.Message, ShouldEqual, "unspecified error")
			})

			Convey(`Will augment POST data with query parameters.`, func() {
				_, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/post?T=bar"),
					"application/json", bytes.NewBufferString(`{"S":"foo"}`))
				So(err, ShouldBeNil)

				So(be.body.String(), ShouldEqual, `{"S":"foo","T":"bar"}`+"\n")
			})

			Convey(`Will return an error if an invalid endpoint is requested.`, func() {
				resp, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/does.not.exist"), "", nil)
				So(err, ShouldBeNil)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusNotFound)
				So(e.Error.Message, ShouldEqual, "unspecified error")
			})

			Convey(`Will catch backend panic() and wrap it with an error.`, func() {
				didPanic := false
				be.callback = func(http.ResponseWriter) {
					didPanic = true
					panic("test panic")
				}

				resp, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/post"), "", nil)
				So(err, ShouldBeNil)

				So(didPanic, ShouldBeTrue)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusServiceUnavailable)
			})

			Convey(`Will catch and forward backend JSON errors.`, func() {
				be.callback = func(w http.ResponseWriter) {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(`{"error_message": "test message"}`))
				}

				resp, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/post"), "", nil)
				So(err, ShouldBeNil)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusNotFound)
				So(e.Error.Message, ShouldEqual, "test message")
			})

			Convey(`Will catch non-JSON backend errors and wrap with generic error.`, func() {
				be.callback = func(w http.ResponseWriter) {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte("$$NOT JSON$$"))
				}

				resp, err := c.Post(fmt.Sprintf("%s%s", ts.URL, "/_ah/api/test/v1/post"), "", nil)
				So(err, ShouldBeNil)

				e := readErrorResponse(resp)
				So(e, ShouldNotBeNil)
				So(e.Error, ShouldNotBeNil)
				So(e.Error.Code, ShouldEqual, http.StatusNotFound)
				So(e.Error.Message, ShouldEqual, "unspecified error")
				So(len(e.Error.Errors), ShouldEqual, 1)
				So(e.Error.Errors[0].Message, ShouldContainSubstring, "Failed to decode error JSON")
			})
		})
	})
}
