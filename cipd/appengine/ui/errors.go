// Copyright 2018 The LUCI Authors.
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

package ui

import (
	"unicode"
	"unicode/utf8"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// renderErr is a handler wrapper converts gRPC errors into HTML pages.
func renderErr(h func(*router.Context) error) router.Handler {
	return func(ctx *router.Context) {
		err := h(ctx)
		if err == nil {
			return
		}

		s, _ := status.FromError(err)
		message := s.Message()
		if message != "" {
			// Convert the first rune to upper case.
			r, n := utf8.DecodeRuneInString(message)
			message = string(unicode.ToUpper(r)) + message[n:]
		} else {
			message = "Unspecified error" // this should not really happen
		}

		ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		ctx.Writer.WriteHeader(grpcutil.CodeStatus(s.Code()))
		templates.MustRender(ctx.Context, ctx.Writer, "pages/error.html", map[string]interface{}{
			"Message": message,
		})
	}
}
