// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"fmt"
	"net/http"
	"strconv"
)

// GetLimit extracts the "limit", "numbuilds", or "num_builds" http param from
// the request, or returns "-1" implying no limit was specified.
func GetLimit(r *http.Request) (int, error) {
	sLimit := r.FormValue("limit")
	if sLimit == "" {
		sLimit = r.FormValue("numbuilds")
		if sLimit == "" {
			sLimit = r.FormValue("num_builds")
			if sLimit == "" {
				return -1, nil
			}
		}
	}
	limit, err := strconv.Atoi(sLimit)
	if err != nil {
		return -1, fmt.Errorf("limit parameter value %q is not a number: %s", sLimit, err)
	}
	if limit < 0 {
		return -1, fmt.Errorf("limit parameter value %q is less than 0", sLimit)
	}
	return limit, nil
}
