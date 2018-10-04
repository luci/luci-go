// Copyright 2017 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/logdog/common/storage"
)

// SigningStorage is an interface to storage used by the Coordinator.
type SigningStorage interface {
	// Storage is the base Storage instance.
	storage.Storage

	// GetSignedURLs attempts to sign the storage's stream's RecordIO archive
	// stream storage URL.
	//
	// If signing is not supported by this Storage instance, this will return
	// a nil signing response and no error.
	GetSignedURLs(context.Context, *URLSigningRequest) (*URLSigningResponse, error)
}

// URLSigningRequest is the set of URL signing parameters passed to a
// SigningStorage.GetSignedURLs call.
type URLSigningRequest struct {
	// Lifetime is the signed URL expiration time.
	Lifetime time.Duration

	// Stream, if true, requests a signed log stream URL.
	Stream bool
	// Index, if true, requests a signed log stream index URL.
	Index bool
}

// HasWork returns true if this signing request actually has work that is
// requested.
func (r *URLSigningRequest) HasWork() bool {
	return (r.Stream || r.Index) && (r.Lifetime > 0)
}

// URLSigningResponse is the resulting signed URLs from a
// SigningStorage.GetSignedURLs call.
type URLSigningResponse struct {
	// Expriation is the signed URL expiration time.
	Expiration time.Time

	// Stream is the signed URL for the log stream, if requested.
	Stream string
	// Index is the signed URL for the log stream index, if requested.
	Index string
}
