// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"context"
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMaster(t *testing.T) {
	c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	datastore.GetTestable(c).Consistent(true)
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).CatchupIndexes()

	Convey(`Tests for master`, t, func() {
		So(putDSMasterJSON(c, &buildbotMaster{
			Name:     "fake",
			Builders: map[string]*buildbotBuilder{"fake": {}},
		}, false), ShouldBeNil)
		So(putDSMasterJSON(c, &buildbotMaster{
			Name:     "fake internal",
			Builders: map[string]*buildbotBuilder{"fake": {}},
		}, true), ShouldBeNil)

		Convey(`GetAllBuilders()`, func() {
			cs, err := GetAllBuilders(c)
			So(err, ShouldBeNil)
			So(len(cs.BuilderGroups), ShouldEqual, 1)
		})
	})
}
