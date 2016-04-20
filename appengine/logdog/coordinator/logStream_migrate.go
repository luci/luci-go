// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"
	"time"

	ds "github.com/luci/gae/service/datastore"
)

func (s *LogStream) migrateSchema() error {
	switch s.Schema {
	case "":
		// Pre-schema versioned schema. Migrate to "1".
		//
		// To do this, we:
		// - See if the ArchiveWhole property is available. If it is, we set the
		//   ArchiveLogEntryCount to equal TerminalIndex+1. Otherwise, we leave
		//   ArchiveLogEntryCount at zero, as there is no (easy) way to determine
		//   its value.

		// Determine archival state.
		if s.Archived() {
			switch props := s.extra["ArchiveWhole"]; len(props) {
			case 0:
				break

			case 1:
				if v, ok := props[0].Value().(bool); ok && v {
					s.ArchiveLogEntryCount = s.TerminalIndex + 1
				}

			default:
				return fmt.Errorf("pre-schema ArchiveWhole has %d values", len(props))
			}
		}

		// Backfill time based on state.
		switch {
		case s.Archived():
			if dt := dateTimeProp(s.extra["ArchivedTime"]); dt.IsZero() {
				s.ArchivedTime = s.Created
			}
			fallthrough

		case s.Terminated():
			if dt := dateTimeProp(s.extra["TerminatedTime"]); dt.IsZero() {
				s.TerminatedTime = s.Created
			}
			fallthrough

		default:
			break
		}

		// Migration complete!
		s.Schema = currentLogStreamSchema
		fallthrough

	case currentLogStreamSchema:
		return nil

	default:
		return fmt.Errorf("unrecognized log stream schema %q", s.Schema)
	}
}

// dateTimeProp returns the time.Time value of a single-property slice, or
// a zero time.Time if the slice wasn't single-value or didn't contain a
// time.Time.
func dateTimeProp(props []ds.Property) time.Time {
	if len(props) == 1 {
		if t, ok := props[0].Value().(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}
