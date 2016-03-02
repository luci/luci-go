// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/proto/google"
)

func loadLogStreamState(ls *coordinator.LogStream) *logdog.LogStreamState {
	lss := logdog.LogStreamState{
		ProtoVersion:  ls.ProtoVersion,
		Created:       google.NewTimestamp(ls.Created),
		Updated:       google.NewTimestamp(ls.Updated),
		TerminalIndex: ls.TerminalIndex,
		Purged:        ls.Purged,
	}
	if ls.Archived() {
		lss.Archive = &logdog.LogStreamState_ArchiveInfo{
			IndexUrl:  ls.ArchiveIndexURL,
			StreamUrl: ls.ArchiveStreamURL,
			DataUrl:   ls.ArchiveDataURL,
			Whole:     ls.ArchiveWhole,
		}
	}

	return &lss
}
