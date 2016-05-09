// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/proto/google"
)

func buildLogStreamState(ls *coordinator.LogStream, lst *coordinator.LogStreamState) *logdog.LogStreamState {
	lss := logdog.LogStreamState{
		ProtoVersion:  ls.ProtoVersion,
		Created:       google.NewTimestamp(ls.Created),
		TerminalIndex: lst.TerminalIndex,
		Purged:        ls.Purged,
	}

	if ast := lst.ArchivalState(); ast.Archived() {
		lss.Archive = &logdog.LogStreamState_ArchiveInfo{
			IndexUrl:      lst.ArchiveIndexURL,
			StreamUrl:     lst.ArchiveStreamURL,
			DataUrl:       lst.ArchiveDataURL,
			Complete:      ast == coordinator.ArchivedComplete,
			LogEntryCount: lst.ArchiveLogEntryCount,
		}
	}

	return &lss
}

func isNoSuchEntity(err error) bool {
	return errors.Any(err, func(err error) bool {
		return err == ds.ErrNoSuchEntity
	})
}
