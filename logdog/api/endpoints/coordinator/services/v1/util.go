// Copyright 2016 The LUCI Authors.
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

package logdog

// GetMessageProject implements ProjectBoundMessage.
func (ar *RegisterStreamRequest) GetMessageProject() string { return ar.Project }

// GetMessageProject implements ProjectBoundMessage.
func (ar *LoadStreamRequest) GetMessageProject() string { return ar.Project }

// GetMessageProject implements ProjectBoundMessage.
func (ar *TerminateStreamRequest) GetMessageProject() string { return ar.Project }

// GetMessageProject implements ProjectBoundMessage.
func (ar *ArchiveStreamRequest) GetMessageProject() string { return ar.Project }

// Complete returns true if the archive request expresses that the archived
// log stream was complete.
//
// A log stream is complete if every entry between zero and its terminal index
// is included.
func (ar *ArchiveStreamRequest) Complete() bool {
	tidx := ar.TerminalIndex
	if tidx < 0 {
		tidx = -1
	}
	return (ar.LogEntryCount == (tidx + 1))
}
