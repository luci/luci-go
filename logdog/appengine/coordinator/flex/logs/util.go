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

package logs

import (
	"go.chromium.org/luci/common/proto/google"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
)

func fillStateFromLogStream(s *logdog.LogStreamState, ls *coordinator.LogStream) {
	s.ProtoVersion = ls.ProtoVersion
	s.Created = google.NewTimestamp(ls.Created)
	s.Purged = ls.Purged
}

func fillStateFromLogStreamState(s *logdog.LogStreamState, lss *coordinator.LogStreamState) {
	s.TerminalIndex = lss.TerminalIndex

	if ast := lss.ArchivalState(); ast.Archived() {
		s.Archive = &logdog.LogStreamState_ArchiveInfo{
			IndexUrl:      lss.ArchiveIndexURL,
			StreamUrl:     lss.ArchiveStreamURL,
			Complete:      ast == coordinator.ArchivedComplete,
			LogEntryCount: lss.ArchiveLogEntryCount,
		}
	}
}

func buildLogStreamState(ls *coordinator.LogStream, lss *coordinator.LogStreamState) *logdog.LogStreamState {
	ret := &logdog.LogStreamState{}
	fillStateFromLogStream(ret, ls)
	fillStateFromLogStreamState(ret, lss)
	return ret
}
