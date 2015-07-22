// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

const (
	// MaxLogEntryDataSize is the maximum size, in bytes, of the data section of
	// a single log entry (1 MiB).
	MaxLogEntryDataSize = 1 * 1024 * 1024

	// MaxDatagramSize is the maximum size, in bytes, of datagram data.
	MaxDatagramSize int = MaxLogEntryDataSize
)
