// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"context"
	"net/http"

	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
)

type testHTTPError int

func (e testHTTPError) Error() string {
	return http.StatusText(int(e))
}

func httpError(s int) error {
	return testHTTPError(s)
}

// testLogsServiceBase is an implementation of logdog.LogsServer that panics
// for each method. It is designed to be embedded in other testing instances.
type testLogsServiceBase struct{}

var _ logdog.LogsServer = (*testLogsServiceBase)(nil)

func (s *testLogsServiceBase) Get(c context.Context, req *logdog.GetRequest) (*logdog.GetResponse, error) {
	panic("not implemented")
}

func (s *testLogsServiceBase) Tail(c context.Context, req *logdog.TailRequest) (*logdog.GetResponse, error) {
	panic("not implemented")
}

func (s *testLogsServiceBase) Query(c context.Context, req *logdog.QueryRequest) (*logdog.QueryResponse, error) {
	panic("not implemented")
}
