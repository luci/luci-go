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

package service

import (
	"net/http"
	"strings"

	commonAuth "github.com/luci/luci-go/common/auth"
)

type serviceModifyingTransport struct {
	userAgent string
}

func (smt *serviceModifyingTransport) roundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return commonAuth.NewModifyingTransport(base, func(req *http.Request) error {
		// Set a user agent based on our service. If an existing user agent is
		// specified, add it onto the end of the string.
		parts := make([]string, 0, 2)
		for _, part := range []string{
			smt.userAgent,
			req.UserAgent(),
		} {
			if part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			req.Header.Set("User-Agent", strings.Join(parts, " / "))
		}
		return nil
	})
}
