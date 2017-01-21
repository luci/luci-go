// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
