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

package prod

import (
	"net/http"

	uf "go.chromium.org/gae/service/urlfetch"
	"golang.org/x/net/context"
	"google.golang.org/appengine/urlfetch"
)

// useURLFetch adds a http.RoundTripper implementation to the context.
func useURLFetch(c context.Context) context.Context {
	return uf.SetFactory(c, func(ci context.Context) http.RoundTripper {
		return &urlfetch.Transport{Context: getAEContext(ci)}
	})
}
