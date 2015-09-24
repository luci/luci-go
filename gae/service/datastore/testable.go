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

	// AutoIndex controls the index creation behavior. If it is set to true, then
	// any time the datastore encounters a missing index, it will silently create
	// one and allow the query to succeed. If it's false, then the query will
	// return an error describing the index which could be added with AddIndexes.
	//
	// By default this is false.
	AutoIndex(bool)

	// DisableSpecialEntities turns off maintenance of special __entity_group__
	// type entities. By default this mainenance is enabled, but it can be
	// disabled by calling this with true.
	//
	// If it's true:
	//   - AllocateIDs returns an error.
	//   - Put'ing incomplete Keys returns an error.
	//   - Transactions are disabled and will return an error.
	//
	// This is mainly only useful when using an embedded in-memory datastore as
	// a fully-consistent 'datastore-lite'. In particular, this is useful for the
	// txnBuf filter which uses it to fulfil queries in a buffered transaction,
	// but never wants the in-memory versions of these entities to bleed through
	// to the user code.
	DisableSpecialEntities(bool)
}
