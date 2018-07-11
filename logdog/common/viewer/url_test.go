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

package viewer

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetURL(t *testing.T) {
	t.Parallel()

	Convey(`Testing viewer URL generation`, t, func() {
		for _, tc := range []struct {
			host    string
			project types.ProjectName
			paths   []types.StreamPath
			url     string
		}{
			{"example.appspot.com", "test", []types.StreamPath{"foo/bar/+/baz"},
				"https://example.appspot.com/logs/test/foo/bar/+/baz"},
			{"example.appspot.com", "test", []types.StreamPath{"foo/bar/+/baz", "qux/+/quux"},
				"https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz&s=test%2Fqux%2F%2B%2Fquux"},
		} {
			Convey(fmt.Sprintf(`Can generate a URL for host %q, project %q, paths %q: [%s]`, tc.host, tc.project, tc.paths, tc.url), func() {
				So(GetURL(tc.host, tc.project, tc.paths...), ShouldEqual, tc.url)
			})
		}
	})
}
