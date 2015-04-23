// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
	"testing"

	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/maruel/ut"
)

func TestArchiver(t *testing.T) {
	t.Parallel()
	client := isolatedclient.New("https://localhost:1", "default")
	a := New(client)
	ut.AssertEqual(t, &Stats{[]int64{}, []int64{}, []*UploadStat{}}, a.Stats())
}
