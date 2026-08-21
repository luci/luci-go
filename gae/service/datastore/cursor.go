// Copyright 2026 The LUCI Authors.
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

package datastore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	mc "go.chromium.org/luci/gae/service/datastore/internal/protos/multicursor"
)

// Cursor is a high-level cursor from QueryIter[V]. This may be comprised of
// one or more RawCursors, corresponding to the underlying queries.
type Cursor []RawCursor

func decodeBase64(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// String encodes the cursor into a form that can be passed back to the RPC
// caller and later decoded with [DecodeCursor].
func (c Cursor) String() string {
	if c == nil {
		panic("cannot serialize nil Cursor")
	}

	// TODO: Encrypt this.

	// If Cursor only contains one RawCursor, return it directly. Our DecodeCursor
	// routine can decode either single or multi-cursor as appropriate, but older
	// versions of the code can *only* decode RawCursors.
	if len(c) == 1 {
		return c[0].String()
	}

	encoded := make([]string, len(c))
	for i, raw := range c {
		if raw != nil {
			encoded[i] = raw.String()
		}
	}
	bytes, _ := proto.Marshal(&mc.Cursors{
		Cursors:     encoded,
		MagicNumber: multiCursorMagic,
		Version:     multiCursorVersion,
	})
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// DecodeCursor converts a string returned by a Cursor into a Cursor instance.
// It will return an error if the supplied string is not valid, or could not
// be decoded by the implementation.
func DecodeCursor(ctx context.Context, s string) (Cursor, error) {
	// TODO: Decrypt this.
	cursBuf, b64err := decodeBase64(s)

	var curs mc.Cursors
	protoErr := proto.Unmarshal(cursBuf, &curs)

	if b64err == nil && protoErr == nil && curs.GetMagicNumber() == multiCursorMagic {
		// This is a multi cursor.
		if curs.Version != multiCursorVersion {
			return nil, fmt.Errorf(
				"Cursor version mismatch. Need %v, got %v", multiCursorVersion, curs.Version)
		}

		rawIface := Raw(ctx)

		decodedCursors := make(Cursor, len(curs.Cursors))
		var errs []error
		for i, cursor := range curs.Cursors {
			if cursor == "" {
				continue
			}
			decoded, err := rawIface.DecodeCursor(cursor)
			if err != nil {
				errs = append(errs, err)
			} else {
				decodedCursors[i] = decoded
			}
		}
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return decodedCursors, nil
	}

	// Cannot be a multiCursor - fall back to raw for now.
	single, err := Raw(ctx).DecodeCursor(s)
	if err != nil {
		return nil, err
	}
	return Cursor{single}, nil
}
