// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
)

// response is a pRPC server response.
// All pRPC responses must be written using write.
type response struct {
	code   codes.Code // defaults to OK
	status int        // defaults to status derived from code.
	header http.Header
	body   []byte
}

// errResponse creates a response with an error.
func errResponse(code codes.Code, status int, format string, a ...interface{}) *response {
	return &response{
		code:   code,
		status: status,
		header: http.Header{
			headerContentType: []string{"text/plain"},
		},
		body: []byte(fmt.Sprintf(format+"\n", a...)),
	}
}

// escapeFmt escapes format characters in a string destined for a format
// parameter. This is used to sanitize externally-supplied strings that are
// passed verbatim into errResponse.
func escapeFmt(s string) string {
	return strings.Replace(s, "%", "%%", -1)
}

// write writes r to w.
func (r *response) write(c context.Context, w http.ResponseWriter) {
	body := r.body
	switch r.code {
	case codes.Internal, codes.Unknown:
		// res.body is error message.
		logging.Fields{
			"code": r.code,
		}.Errorf(c, "%s", body)
		body = []byte("Internal Server Error\n")
	}

	for h, vs := range r.header {
		w.Header()[h] = vs
	}
	w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(r.code)))

	status := r.status
	if status == 0 {
		status = codeStatus(r.code)
	}
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		logging.WithError(err).Errorf(c, "Could not respond")
		// The header is already written. There is nothing more we can do.
		return
	}
}
