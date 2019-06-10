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

package storage

import (
	"context"
	"errors"
	"time"

	"go.chromium.org/luci/logdog/common/types"
)

var (
	// ErrExists returned if an attempt is made to overwrite an existing record.
	ErrExists = errors.New("storage: record exists")

	// ErrDoesNotExist returned if an attempt is made to read a record that
	// doesn't exist.
	ErrDoesNotExist = errors.New("storage: record does not exist")

	// ErrBadData is an error returned when the stored data is invalid.
	ErrBadData = errors.New("storage: bad data")

	// ErrReadOnly can be returned by Storage methods to indicate that the Storage
	// is read-only.
	ErrReadOnly = errors.New("storage: read only")
)

// Config is the set of runtime configuration parameters for this storage
// instance.
type Config struct {
	// MaxLogAge is the maximium amount of time that a log entry must be kept for.
	// The Storage instance is welcome to delete any log entries that exceed this
	// age.
	MaxLogAge time.Duration
}

// PutRequest describes adding a single storage record to BigTable.
type PutRequest struct {
	// Project is the project name of the stream.
	Project types.ProjectName
	// Path is the stream path to retrieve.
	Path types.StreamPath
	// Index is the entry's stream index.
	Index types.MessageIndex

	// Values are contiguous sequential records to add to the storage. The first
	// index in values corresponds to Index.
	Values [][]byte
}

// GetRequest is a request to retrieve a series of LogEntry records.
type GetRequest struct {
	// Project is the project name of the stream.
	Project types.ProjectName
	// Path is the stream path to retrieve.
	Path types.StreamPath
	// Index is the entry's stream index.
	Index types.MessageIndex

	// Limit is the maximum number of records to return before stopping iteration.
	// If <= 0, no maximum limit will be applied.
	//
	// The Storage instance may return fewer records than the supplied Limit as an
	// implementation detail.
	Limit int
	// KeysOnly, if true, allows (but doesn't require) the Storage instance to
	// omit entry data in its get callback. For scanning operations, this can be
	// much cheaper/faster than full data queries.
	KeysOnly bool
}

// GetCallback is invoked for each record in the Get request. If it returns
// false, iteration should stop.
//
// The MessageIndex may be -1 if the message index isn't known. In this case,
// the caller will have to unmarshal the log entry data to determine its index.
type GetCallback func(*Entry) bool

// Storage is an abstract LogDog storage implementation. Interfaces implementing
// this may be used to store and retrieve log records by the collection service
// layer.
//
// All of these methods must be synchronous and goroutine-safe.
//
// All methods may return errors.Transient errors if they encounter an error
// that may be transient.
type Storage interface {
	// Close shuts down this instance, releasing any allocated resources.
	Close()

	// Writes log record data to storage.
	//
	// If the data already exists, ErrExists will be returned.
	Put(context.Context, PutRequest) error

	// Get invokes a callback over a range of sequential LogEntry records.
	//
	// These log entries will be returned in order (e.g., seq(Rn) < seq(Rn+1)),
	// but, depending on ingest, may not be contiguous.
	//
	// The underlying Storage implementation may return fewer records than
	// requested based on availability or implementation details; consequently,
	// receiving fewer than requested records does not necessarily mean that more
	// records are not available.
	//
	// Returns nil if retrieval executed successfully, ErrDoesNotExist if
	// the requested stream does not exist, and an error if an error occurred
	// during retrieval.
	Get(context.Context, GetRequest, GetCallback) error

	// Tail retrieves the latest log in the stream. If the stream has no logs, it
	// will fail with ErrDoesNotExist.
	//
	// The MessageIndex may be -1 if the message index isn't known. In this case,
	// the caller will have to unmarshal the log entry data to determine its
	// index.
	Tail(context.Context, types.ProjectName, types.StreamPath) (*Entry, error)

	// Config installs the supplied configuration parameters into the storage
	// instance.
	Config(context.Context, Config) error
}
