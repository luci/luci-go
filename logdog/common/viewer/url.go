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

// Package viewer is a support library to interact with the LogDog web app and
// log stream viewer.
package viewer

import (
	"fmt"
	"net/url"

	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
)

// GetURL generates a LogDog app viewer URL for the specified streams.
func GetURL(host string, project cfgtypes.ProjectName, paths ...types.StreamPath) string {
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
