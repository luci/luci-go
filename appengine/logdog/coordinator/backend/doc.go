// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package backend implements the set of backend handlers used by the LogDog
// Coordiantor. This consists of both cron handlers and work handlers. The
// cron handlers will dispatch task queue tasks that will be serviced by the
// work handlers.
//
// The backend interfaces between the two log storage spaces that LogDog uses:
//	- Intermediate storage: Logs are accumulated here from the Transport by the
//	  Collector. They reside here until they are either complete or they
//	  timeout, at which point they are moved (via archival) to the archive
//	  storage.
//	- Archive storage: Logs are moved from intermediate storage to archive
//	  storage upon completion. An index and associated data is generated for the
//	  stream and it is considered finalized and immutable. This should be a
//	  cheaper storage location, possibly with cold storage capabilities.
//
// Archival
//
// Archival begins with a periodic cron job that scans through LogStream
// datastore entries looking for streams that have not yet been archived. A
// given stream will have archival initiated if:
//	- It has been closed for the configured `archive_delay` period.
//	- It has not been archived for at least `archive_max_delay`.
//
// In the first case, we scan for logs that have been terminated and dispatch
// archival tasks requesting complete archival. This is the standard case,
// and will identify log streams that have had their terminal indexes
// registered.
//
// The second case is the failsafe case. If a log stream has been inactive for
// sufficiently long enough without actually being terminal, we preempt it and
// assume that something weng wrong in transit, dropping the terminal log entry.
//
// The archive cron job will dispatch an archive request to the archive backend
// handler for each log stream that matches one of these situations.
//
// Each archival task will look at the last time the LogStream has been updated.
// If this does not exceed our `archive_max_delay` (standard case), we will only
// complete archival if every LogEntry between [0..terminalIndex] is
// successfully archived. If we are past `archive_max_delay` (failsafe), we will
// do a best-effort sparse archival with whatever data is available.
package backend
