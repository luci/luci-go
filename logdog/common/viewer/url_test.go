// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package viewer

import (
	"fmt"
	"testing"

	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetURL(t *testing.T) {
	t.Parallel()

	Convey(`Testing viewer URL generation`, t, func() {
		for _, tc := range []struct {
			host    string
			project cfgtypes.ProjectName
			paths   []types.StreamPath
			url     string
		}{
			{"example.appspot.com", "test", []types.StreamPath{"foo/bar/+/baz"},
				"https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz"},
			{"example.appspot.com", "test", []types.StreamPath{"foo/bar/+/baz", "qux/+/quux"},
				"https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz&s=test%2Fqux%2F%2B%2Fquux"},
			{"example.appspot.com", "test", []types.StreamPath{"query/*/+/**"},
				"https://example.appspot.com/v/?s=test%2Fquery%2F%2A%2F%2B%2F%2A%2A"},
		} {
			Convey(fmt.Sprintf(`Can generate a URL for host %q, project %q, paths %q: [%s]`, tc.host, tc.project, tc.paths, tc.url), func() {
				So(GetURL(tc.host, tc.project, tc.paths...), ShouldEqual, tc.url)
			})
		}
	})
}
