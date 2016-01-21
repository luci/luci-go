// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// "now" for our tests.
var now = time.Date(2015, 1, 1, 0, 0, 0, 0, time.Local)

type testHTTPError int

func (e testHTTPError) Error() string {
	return http.StatusText(int(e))
}

func httpError(s int) error {
	return testHTTPError(s)
}

// testRT is an http.RoundTripper implementation used for testing.
type testRT struct {
	handler func(req *http.Request) (interface{}, error)
}

func (t *testRT) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := http.Response{}
	if t.handler == nil {
		return &resp, nil
	}

	d, err := t.handler(req)
	if err != nil {
		switch t := err.(type) {
		case testHTTPError:
			resp.StatusCode = int(t)
			resp.Body = ioutil.NopCloser(bytes.NewBuffer([]byte("{}")))
			return &resp, nil
		default:
			return nil, err
		}
	}
	if d == nil {
		d = map[string]interface{}{}
	}

	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(d); err != nil {
		return nil, err
	}

	resp.Header = http.Header{}
	resp.Header.Add("content-type", "application/json")
	resp.Body = ioutil.NopCloser(&buf)
	resp.StatusCode = http.StatusOK
	return &resp, nil
}

func gen(name string, pb *logpb.LogStreamDescriptor, state *logs.LogStreamState) *logs.QueryResponseStream {
	qrs := logs.QueryResponseStream{
		Path:  fmt.Sprintf("test/+/%s", name),
		State: state,
	}

	if pb != nil {
		d, err := proto.Marshal(&logpb.LogStreamDescriptor{
			Prefix: "test",
			Name:   name,
		})
		impossible(err)
		qrs.DescriptorProto = base64.StdEncoding.EncodeToString(d)
	}

	return &qrs
}

func b64Proto(pb proto.Message) string {
	d, err := proto.Marshal(pb)
	impossible(err)
	return base64.StdEncoding.EncodeToString(d)
}

func timeString(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func impossible(err error) {
	if err != nil {
		panic(err)
	}
}
