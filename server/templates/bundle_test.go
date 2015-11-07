// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package templates

import (
	"bytes"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRender(t *testing.T) {
	assets := map[string]string{
		"includes/base": `
{{define "base"}}
base
{{template "content" .}}
{{end}}
`,
		"pages/page": `
{{define "content"}}
content {{.arg1}} {{.arg2}}
{{end}}
`,
	}

	Convey("Works", t, func() {
		b := Bundle{
			Loader:          AssetsLoader(assets),
			DefaultTemplate: "base",
			DefaultArgs: func(c context.Context) (Args, error) {
				return Args{"arg1": "val1"}, nil
			},
		}
		c := Use(context.Background(), &b)

		tmpl, err := Get(c, "pages/page")
		So(tmpl, ShouldNotBeNil)
		So(err, ShouldBeNil)

		blob, err := Render(c, "pages/page", Args{"arg2": "val2"})
		So(err, ShouldBeNil)
		So(string(blob), ShouldEqual, "\nbase\n\ncontent val1 val2\n\n")

		buf := bytes.Buffer{}
		MustRender(c, &buf, "pages/page", Args{"arg2": "val2"})
		So(buf.String(), ShouldEqual, "\nbase\n\ncontent val1 val2\n\n")
	})
}
