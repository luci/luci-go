// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate stringer -type=Toggle

package datastore

// GeoPoint represents a location as latitude/longitude in degrees.
//
// You probably shouldn't use these, but their inclusion here is so that the
// datastore service can interact (and round-trip) correctly with other
// datastore API implementations.
type GeoPoint struct {
	Lat, Lng float64
}

// Valid returns whether a GeoPoint is within [-90, 90] latitude and [-180,
// 180] longitude.
func (g GeoPoint) Valid() bool {
	return -90 <= g.Lat && g.Lat <= 90 && -180 <= g.Lng && g.Lng <= 180
}

// TransactionOptions are the options for running a transaction.
type TransactionOptions struct {
	// XG is whether the transaction can cross multiple entity groups. In
	// comparison, a single group transaction is one where all datastore keys
	// used have the same root key. Note that cross group transactions do not
	// have the same behavior as single group transactions. In particular, it
	// is much more likely to see partially applied transactions in different
	// entity groups, in global queries.
	// It is valid to set XG to true even if the transaction is within a
	// single entity group.
	XG bool
	// Attempts controls the number of retries to perform when commits fail
	// due to a conflicting transaction. If omitted, it defaults to 3.
	Attempts int
}

// Toggle is a tri-state boolean (Auto/True/False), which allows structs
// to control boolean flags for metadata in a non-ambiguous way.
type Toggle byte

// These are the allowed values for Toggle. Any other values are invalid.
const (
	Auto Toggle = iota
	On
	Off
)
