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

package engine

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/internal"
)

// invocationsCursor is used to paginate ListInvocations results.
//
// It wraps cursors.InvocationsCursor proto.
type invocationsCursor struct {
	QueryCursor datastore.Cursor
}

// decodeInvocationsCursor deserializes the cursor.
func decodeInvocationsCursor(c context.Context, cursor string) (cur invocationsCursor, err error) {
	if cursor == "" {
		return
	}

	blob, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		err = errors.Annotate(err, "failed to base64 decode the cursor").Err()
		return
	}

	msg := internal.InvocationsCursor{}
	if err = proto.Unmarshal(blob, &msg); err != nil {
		err = errors.Annotate(err, "failed to unmarshal the cursor").Err()
		return
	}

	if len(msg.DsCursor) != 0 {
		// GAE lib wants the cursor base64-encoded.
		dsCurb64 := base64.RawURLEncoding.EncodeToString(msg.DsCursor)
		if cur.QueryCursor, err = datastore.DecodeCursor(c, dsCurb64); err != nil {
			err = errors.Annotate(err, "failed to decode datastore cursor").Err()
			return
		}
	}

	return
}

// Serialize converts the cursor to an URL-safe string.
func (cur *invocationsCursor) Serialize() (str string, err error) {
	if cur.QueryCursor == nil {
		return
	}

	msg := internal.InvocationsCursor{}

	// GAE lib encodes the cursor in base64.RawURLEncoding. Unwrap it back, to put
	// raw bytes into our proto, to avoid double base64 encoding.
	msg.DsCursor, err = base64.RawURLEncoding.DecodeString(cur.QueryCursor.String())
	if err != nil {
		return // must never actually happen
	}

	blob, err := proto.Marshal(&msg)
	if err != nil {
		return // also must never actually happen
	}

	return base64.RawURLEncoding.EncodeToString(blob), nil
}
