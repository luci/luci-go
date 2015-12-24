// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package endpoints

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
)

// LogStreamState is a bidirectional state value used in UpdateStream calls.
//
// LogStreamState is embeddable in Endpoints request/response structs.
type LogStreamState struct {
	// ProtoVersion is the protobuf version for this stream.
	ProtoVersion string `json:"protoVersion,omitempty"`

	// Created is the time, represented as a UTC RFC3339 string, when the log
	// stream was created.
	Created string `json:"created,omitempty"`
	// Updated is the time, represented as a UTC RFC3339 string, when the log
	// stream was last updated.
	Updated string `json:"updated,omitempty"`

	// TerminalIndex contains the stream index of the log stream's terminal message.
	// If the value is -1, the log is still streaming.
	TerminalIndex int64 `json:"terminalIndex,string,omitempty"`

	// ArchiveIndexURL is the Google Storage URL where the log stream's index is
	// archived.
	ArchiveIndexURL string `json:"archiveIndexURL,omitempty"`
	// ArchiveStreamURL is the Google Storage URL where the log stream's raw
	// stream data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveStreamURL string `json:"archiveStreamURL,omitempty"`
	// ArchiveDataURL is the Google Storage URL where the log stream's assembled
	// data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveDataURL string `json:"archiveDataURL,omitempty"`

	// Purged indicates the purged state of a log. A log that has been purged is
	// only acknowledged to administrative clients.
	Purged bool `json:"purged,omitempty"`
}

// LoadLogStreamState loads the contents of a Datastore LogStream record into an
// endpoint LogStream structure.
//
// If desc is true, deserialize the LogStream's LogStreamDescriptor protobuf
// into the Descriptor field.
func LoadLogStreamState(ls *coordinator.LogStream) *LogStreamState {
	return &LogStreamState{
		ProtoVersion:     ls.ProtoVersion,
		Created:          ToRFC3339(ls.Created),
		Updated:          ToRFC3339(ls.Updated),
		TerminalIndex:    ls.TerminalIndex,
		ArchiveIndexURL:  ls.ArchiveIndexURL,
		ArchiveStreamURL: ls.ArchiveStreamURL,
		ArchiveDataURL:   ls.ArchiveDataURL,
		Purged:           ls.Purged,
	}
}
