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
