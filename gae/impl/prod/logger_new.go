// Copyright 2022 The LUCI Authors.
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

//go:build go1.12
// +build go1.12

package prod

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/sdlogger"
)

// Disable calls to LogFlush RPC from within google.golang.org/appengine guts,
// this RPC is not available on second gen runtime. This code requires running
// https://github.com/golang/appengine/commit/07f9b0860d0 or newer, which
// unfortunately wasn't released as a tagged version.
func init() {
	if err := os.Setenv("LOG_TO_LOGSERVICE", "0"); err != nil {
		panic(fmt.Sprintf("Error setting LOG_TO_LOGSERVICE=0: %s", err))
	}
}

var logSink = &sdlogger.Sink{Out: os.Stdout}

// useLogging adds a logging.Logger implementation to the context which emits
// structured logs to stdout in a form GAE runtime can parse.
func useLogging(c context.Context, req *http.Request) context.Context {
	traceID, spanID := traceAndSpan(req)
	return logging.SetFactory(c, sdlogger.Factory(logSink, sdlogger.LogEntry{
		TraceID:   traceID,
		SpanID:    spanID,
		Operation: &sdlogger.Operation{ID: genUniqueID(32)},
	}, nil))
}

var traceContextRe = regexp.MustCompile(`^(\w+)/(\d+)(?:;o=[01])?$`)

func traceAndSpan(req *http.Request) (string, string) {
	headers := req.Header["X-Cloud-Trace-Context"]
	if len(headers) < 1 {
		return "", ""
	}
	matches := traceContextRe.FindAllStringSubmatch(headers[0], -1)
	if len(matches) < 1 || len(matches[0]) < 3 {
		return "", ""
	}
	traceID := matches[0][1]
	spanID := matches[0][2]
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	return fmt.Sprintf("projects/%s/traces/%s", projectID, traceID), spanID
}

func genUniqueID(n int) string {
	b := make([]byte, n/2)
	rand.Read(b) // note: doesn't have to be crypto random
	return hex.EncodeToString(b)
}
