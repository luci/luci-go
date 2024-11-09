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

package coordinator

import (
	"context"
	"errors"
	"fmt"
	"time"

	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/logdog/common/types"
)

// ArchivalState describes the archival state of a LogStream.
type ArchivalState int

const (
	// NotArchived means that the stream is not archived, and that no archival has
	// been tasked.
	NotArchived ArchivalState = iota
	// ArchiveTasked is true if the log stream has an archival tasked, but has
	// not yet been archived.
	ArchiveTasked
	// ArchivedPartial means that the stream is archived, but that some log
	// entries are missing.
	ArchivedPartial
	// ArchivedComplete means that the stream is archived and all log entries are
	// present.
	ArchivedComplete
)

// Archived returns true if this ArchivalState implies that the log stream is
// archived.
func (as ArchivalState) Archived() bool {
	switch as {
	case ArchivedPartial, ArchivedComplete:
		return true
	default:
		return false
	}
}

// ArchivalStateKey is the name of the index key for the archival state.
var ArchivalStateKey = "_ArchivalState"

// LogStreamStateExpiry is the duration after creation that a LogStreamState
// record should persist for.  After this duration it may be deleted.
const LogStreamStateExpiry = 540 * 24 * time.Hour

// LogStreamState contains the current state of a LogStream.
//
// This structure has additional datastore fields imposed by the
// PropertyLoadSaver.
//   - _Terminated is true if the LogStream has been terminated.
//   - _ArchivePending is true if the LogStream currently has an archive task
//     dispatched.
//   - _ArchivalState is true if the LogStream has been archived.
//
// See services API's LogStreamState message type.
type LogStreamState struct {
	_id int `gae:"$id,1"`
	// Parent is the key of the corresponding LogStream.
	Parent *ds.Key `gae:"$parent"`

	// Schema is the datastore schema version for this object. This can be used
	// to facilitate schema migrations.
	//
	// The current schema is CurrentSchemaVersion.
	Schema string `gae:",noindex"`

	// Created is the last time that this state has been created.
	Created time.Time `gae:",noindex"`
	// Updated is the last time that this state has been updated.
	Updated time.Time `gae:",noindex"`
	// ExpireAt is time after which the state will be deleted.
	ExpireAt time.Time `gae:",noindex"`

	// Secret is the Butler secret value for this stream.
	//
	// This value may only be returned to LogDog services; it is not user-visible.
	Secret []byte `gae:",noindex"`

	// TerminatedTime is the Coordinator's record of when this log stream was
	// terminated.
	TerminatedTime time.Time `gae:",noindex"`
	// TerminalIndex is the index of the last log entry in the stream.
	//
	// If this is <0, the log stream is either still streaming or has been
	// archived with no log entries.
	TerminalIndex int64 `gae:",noindex"`

	// ArchiveRetryCount is the number of times this stream has attempted
	// archival.
	ArchiveRetryCount int64

	// ArchivedTime is the Coordinator's record of when this log stream was
	// archived. If this is non-zero, it means that the log entry has been
	// archived.
	ArchivedTime time.Time `gae:",noindex"`
	// ArchiveLogEntryCount is the number of LogEntry records that were archived
	// for this log stream.
	//
	// This is valid only if the log stream is Archived.
	ArchiveLogEntryCount int64 `gae:",noindex"`
	// ArchivalKey is the archival key for this log stream. This is used to
	// differentiate the real archival request from those that were dispatched,
	// but that ultimately failed to update state.
	//
	// See createArchivalKey for details on its generation and usage.
	ArchivalKey []byte `gae:",noindex"`

	// ArchiveIndexURL is the Google Storage URL where the log stream's index is
	// archived.
	ArchiveIndexURL string `gae:",noindex"`
	// ArchiveIndexSize is the size, in bytes, of the archived Index. It will be
	// zero if the file is not archived.
	ArchiveIndexSize int64 `gae:",noindex"`
	// ArchiveStreamURL is the Google Storage URL where the log stream's raw
	// stream data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveStreamURL string `gae:",noindex"`
	// ArchiveStreamSize is the size, in bytes, of the archived stream. It will be
	// zero if the file is not archived.
	ArchiveStreamSize int64 `gae:",noindex"`

	// extra causes datastore to ignore unrecognized fields and strip them in
	// future writes.
	extra ds.PropertyMap `gae:"-,extra"`
}

// NewLogStreamState returns a LogStreamState with its parent key populated to
// the LogStream with the supplied ID.
func NewLogStreamState(c context.Context, id HashID) *LogStreamState {
	return &LogStreamState{Parent: ds.NewKey(c, "LogStream", string(id), 0, nil)}
}

// ID returns the LogStream ID for the LogStream that owns this LogStreamState.
func (lst *LogStreamState) ID() HashID {
	return HashID(lst.Parent.StringID())
}

// Load implements ds.PropertyLoadSaver.
func (lst *LogStreamState) Load(pmap ds.PropertyMap) error {
	// Discard derived properties.
	delete(pmap, "_Terminated")
	delete(pmap, ArchivalStateKey)

	return ds.GetPLS(lst).Load(pmap)
}

// Save implements ds.PropertyLoadSaver.
func (lst *LogStreamState) Save(withMeta bool) (ds.PropertyMap, error) {
	lst.Schema = CurrentSchemaVersion

	// Save default struct fields.
	pmap, err := ds.GetPLS(lst).Save(withMeta)
	if err != nil {
		return nil, err
	}

	pmap["_Terminated"] = ds.MkProperty(lst.Terminated())
	pmap[ArchivalStateKey] = ds.MkProperty(lst.ArchivalState())

	return pmap, nil
}

// Validate evaluates the state and data contents of the LogStreamState and
// returns an error if it is invalid.
func (lst *LogStreamState) Validate() error {
	if lst.Created.IsZero() {
		return errors.New("missing created time")
	}
	if lst.Updated.IsZero() {
		return errors.New("missing updated time")
	}

	if lst.Terminated() && lst.TerminatedTime.IsZero() {
		return errors.New("log stream is terminated, but missing terminated time")
	}
	if err := types.PrefixSecret(lst.Secret).Validate(); err != nil {
		return fmt.Errorf("invalid prefix secret: %v", err)
	}

	return nil
}

// Terminated returns true if this stream has been terminated.
func (lst *LogStreamState) Terminated() bool {
	if lst.ArchivalState().Archived() {
		return true
	}
	return lst.TerminalIndex >= 0
}

// ArchivalState returns the archival state of the log stream.
func (lst *LogStreamState) ArchivalState() ArchivalState {
	if lst.ArchivedTime.IsZero() {
		// Not archived, have we dispatched an archival?
		if len(lst.ArchivalKey) > 0 {
			return ArchiveTasked
		}
		return NotArchived
	}

	if lst.ArchiveLogEntryCount > lst.TerminalIndex {
		return ArchivedComplete
	}
	return ArchivedPartial
}
