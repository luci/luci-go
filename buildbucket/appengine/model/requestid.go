// Copyright 2021 The LUCI Authors.
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

package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
)

// RequestID stores request IDs for request deduplication.
type RequestID struct {
	_     datastore.PropertyMap `gae:"-,extra"`
	_kind string                `gae:"$kind,RequestID"`
	// ID is a string of the form "<auth.Identity>:<request ID string>" encoded
	// as a hex string using SHA-256 for a well-distributed key space.
	ID string `gae:"$id"`

	// BuildID is the ID of the Build entity this entity refers to.
	BuildID    int64             `gae:"build_id,noindex"`
	CreatedBy  identity.Identity `gae:"created_by,noindex"`
	CreateTime time.Time         `gae:"create_time,noindex"`
	// RequestID is the original request ID string this entity was created from.
	RequestID string `gae:"request_id,noindex"`
}

// NewRequestID returns a request ID with the entity ID filled in.
func NewRequestID(ctx context.Context, buildID int64, now time.Time, requestID string) *RequestID {
	u := auth.CurrentIdentity(ctx)
	return &RequestID{
		ID:         fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s", u, requestID)))),
		BuildID:    buildID,
		CreatedBy:  u,
		CreateTime: now,
		RequestID:  requestID,
	}
}
