// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"net/http"

	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"golang.org/x/net/context"
)

type testHTTPError int

func (e testHTTPError) Error() string {
	return http.StatusText(int(e))
}

func httpError(s int) error {
	return testHTTPError(s)
}

// testLogsServiceBase is an implementation of logs.LogsServer that panics
// for each method. It is designed to be embedded in other testing instances.
type testLogsServiceBase struct{}

var _ logs.LogsServer = (*testLogsServiceBase)(nil)

func (s *testLogsServiceBase) Get(c context.Context, req *logs.GetRequest) (*logs.GetResponse, error) {
	panic("not implemented")
}

func (s *testLogsServiceBase) Tail(c context.Context, req *logs.TailRequest) (*logs.GetResponse, error) {
	panic("not implemented")
}

func (s *testLogsServiceBase) Query(c context.Context, req *logs.QueryRequest) (*logs.QueryResponse, error) {
	panic("not implemented")
}
