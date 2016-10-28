// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package viewer is a support library to interact with the LogDog web app and
// log stream viewer.
package viewer

import (
	"fmt"
	"net/url"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/logdog/common/types"
)

// GetURL generates a LogDog app viewer URL for the specified streams.
func GetURL(host string, project config.ProjectName, paths ...types.StreamPath) string {
	values := make([]string, len(paths))
	for i, p := range paths {
		values[i] = fmt.Sprintf("%s/%s", project, p)
	}
	u := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "v/",
		RawQuery: url.Values{
			"s": values,
		}.Encode(),
	}
	return u.String()
}
