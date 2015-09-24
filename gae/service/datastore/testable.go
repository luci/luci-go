// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

// TestingSnapshot is an opaque implementation-defined snapshot type.
type TestingSnapshot interface {
	ImATestingSnapshot()
}

// Testable is the testable interface for fake datastore implementations.
type Testable interface {
	// AddIndex adds the provided index.
	// Blocks all datastore access while the index is built.
	// Panics if any of the IndexDefinition objects are not Compound()
	AddIndexes(...*IndexDefinition)

	// TakeIndexSnapshot allows you to take a snapshot of the current index
	// tables, which can be used later with SetIndexSnapshot.
	TakeIndexSnapshot() TestingSnapshot

	// SetIndexSnapshot allows you to set the state of the current index tables.
	// Note that this would allow you to create 'non-lienarities' in the precieved
	// index results (e.g. you could force the indexes to go back in time).
	//
	// SetIndexSnapshot takes a reference of the given TestingSnapshot. You're
	// still responsible for closing the snapshot after this call.
	SetIndexSnapshot(TestingSnapshot)

	// CatchupIndexes catches the index table up to the current state of the
	// datastore. This is equivalent to:
	//   idxSnap := TakeIndexSnapshot()
	//   SetIndexSnapshot(idxSnap)
	//
	// But depending on the implementation it may implemented with an atomic
	// operation.
	CatchupIndexes()

	// SetTransactionRetryCount set how many times RunInTransaction will retry
	// transaction body pretending transaction conflicts happens. 0 (default)
	// means commit succeeds on the first attempt (no retries).
	SetTransactionRetryCount(int)

	// Consistent controls the eventual consistency behavior of the testing
	// implementation. If it is called with true, then this datastore
	// implementation will be always-consistent, instead of eventually-consistent.
	//
	// By default the datastore is eventually consistent, and you must call
	// CatchupIndexes or use Take/SetIndexSnapshot to manipulate the index state.
	Consistent(always bool)
}
