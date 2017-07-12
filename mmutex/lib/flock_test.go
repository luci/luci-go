// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lib

import (
	"io/ioutil"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "github.com/luci/luci-go/common/testing/assertions"
)

func TestAcquireExclusiveLock(t *testing.T) {
	lockDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Error("received error creating temporary testing directory")
	}
	defer os.Remove(lockDir)
	path := lockDir + "test.lock"

	Convey("AcquireExclusiveLock errors if lock file doesn't exist", t, func(c C) {
		So(AcquireExclusiveLock(path), ShouldErrLike, "lock file does not exist")
	})
}
