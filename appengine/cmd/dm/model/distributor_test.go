// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/display"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDistributor(t *testing.T) {
	t.Parallel()

	Convey("Distributor.ToDisplay", t, func() {
		d := &Distributor{"name", "https://www.example.com"}
		So(d.ToDisplay(), ShouldResemble, &display.Distributor{
			Name: "name", URL: "https://www.example.com"})
	})
}
