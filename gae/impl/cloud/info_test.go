// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cloud

import (
	"fmt"
	"strings"
	"testing"

	"github.com/luci/gae/service/info"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	Convey(`A testing Info service`, t, func() {
		const maxNamespaceLen = 100

		c := useInfo(context.Background())

		Convey(`Can set valid namespaces.`, func() {
			for _, v := range []string{
				"",
				"test",
				"0123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_",
				strings.Repeat("X", maxNamespaceLen),
			} {
				Convey(fmt.Sprintf(`Rejects %q`, v), func() {
					c, err := info.Namespace(c, v)
					So(err, ShouldBeNil)
					So(info.GetNamespace(c), ShouldEqual, v)
				})
			}
		})

		Convey(`Rejects invalid namespaces on the client.`, func() {
			for _, v := range []string{
				" ",
				strings.Repeat("X", maxNamespaceLen+1),
			} {
				Convey(fmt.Sprintf(`Rejects %q`, v), func() {
					_, err := info.Namespace(c, v)
					So(err, ShouldErrLike, "does not match")
				})
			}
		})
	})
}
