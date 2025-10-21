// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"context"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// Visitor is a callback passed to VisitMetadata.
//
// It decides whether to continue exploring this metadata subtree or not.
type Visitor func(prefix string, md []*repopb.PrefixMetadata) (cont bool, err error)

// Storage knows how to store, fetch and update prefix metadata, as well as
// how to calculate its fingerprint.
//
// The metadata is organized in a forest-like structure, where each node is
// associated with some package prefix (e.g. has a name "a/b/c"). The single
// root ("") may or may not be present, depending on the implementation.
//
// It doesn't try to understand what metadata means, just fingerprints, stores
// and enumerates it.
//
// This functionality is organized into an interface to simplify mocking. Use
// GetStorage to grab a real implementation.
type Storage interface {
	// GetMetadata fetches metadata associated with the given prefix and all
	// parent prefixes.
	//
	// The prefix may be an empty string, in which case the root metadata will be
	// returned, if it is defined.
	//
	// Does not check permissions.
	//
	// The return value is sorted by the prefix length. Prefixes without metadata
	// are skipped. For example, when requesting metadata for prefix "a/b/c/d" the
	// return value may contain entries for "", "a", "a/b", "a/b/c/d" (in that
	// order, where "" indicates the root and "a/b/c" is skipped in this example
	// as not having any metadata attached).
	//
	// Note that the prefix of the last entry doesn't necessary match 'prefix'.
	// This happens if metadata for that prefix doesn't exist. Similarly, the
	// returns value may be completely empty slice in case there's no metadata
	// for the requested prefix and all its parent prefixes.
	//
	// Returns a fatal error if the prefix is malformed, all other errors are
	// transient.
	GetMetadata(ctx context.Context, prefix string) ([]*repopb.PrefixMetadata, error)

	// VisitMetadata enumerates the metadata in depth-first order.
	//
	// Can be used to fetch all metadata items at and under the given prefix.
	//
	// The callback is called for each visited node (always starting from 'prefix'
	// itself, even if it has no metadata directly attached to it), receiving same
	// metadata list as if GetMetadata was used to fetch it. The callback can
	// decide whether to proceed with the enumeration of the corresponding subtree
	// or skip it (by returning either true or false).
	//
	// Aborts the traversal on a first error from the callback.
	//
	// Returns either a transient error if fetching failed, or whatever error the
	// callback returned.
	VisitMetadata(ctx context.Context, prefix string, cb Visitor) error

	// UpdateMetadata transactionally (with XG transaction) updates or creates
	// metadata of some prefix and returns it.
	//
	// The prefix may be an empty string, in which case the root metadata will
	// be updated, if allowed.
	//
	// Does not check permissions. Does not check the format of the updated
	// metadata.
	//
	// It fetches the metadata object and calls the callback to modify it. The
	// callback may be called multiple times when retrying the transaction. If the
	// callback mutates the metadata and doesn't return an error, the metadata's
	// fingerprint is updated, the metadata is saved to the storage and returned
	// to the caller.
	//
	// If the metadata object doesn't exist yet, the callback will be called with
	// an empty object that has only 'Prefix' field populated. The callback then
	// can populate the rest of the fields. If it doesn't touch any fields
	// (but still succeeds), UpdateMetadata will return nil PrefixMetadata, to
	// indicate the metadata is still absent.
	//
	// If the callback returns an error, it will be returned as is. If the
	// transaction itself fails, returns a transient error.
	UpdateMetadata(ctx context.Context, prefix string, cb func(ctx context.Context, m *repopb.PrefixMetadata) error) (*repopb.PrefixMetadata, error)
}

// GetStorage returns production implementation of the metadata storage.
func GetStorage() Storage {
	return &legacyStorageImpl{}
}
