// Copyright 2024 The LUCI Authors.
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

// Package cursor contains logic for marshalling pagination cursors.
package cursor

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
)

// cursorAEADContext is used as additional data for AEAD encryption operation.
//
// It makes sure the produced ciphertext can only be successfully decrypted as
// the cursor (i.e. when the decryption operation uses the same context string).
const cursorAEADContext = "swarming-cursor"

var (
	// cursorEncodeErr is a generic error returned to users by Encode.
	cursorEncodeErr = status.Errorf(codes.Internal, "failed to prepare the pagination cursor")
	// cursorDecodeErr is a generic error returned by users by Decode.
	cursorDecodeErr = status.Errorf(codes.InvalidArgument, "bad pagination cursor")
	// noKeyErr is returned if the server has no AEAD key configured.
	noKeyErr = status.Errorf(codes.Aborted, "the server is not properly configured")
)

// Encode serializes, encrypts and base64-encode the cursor.
//
// `payload` is expected to have a type matching `kind` (see RequestKind enum
// in cursor.proto), e.g. if `kind` is LIST_BOTS, `payload` must be
// *BotsCursor. Panics on violations.
//
// Returns gRPC errors. All errors have INTERNAL code (since there's no other
// way for this function to fail) and can be returned to the user as are. Extra
// details are logged.
func Encode(ctx context.Context, kind cursorpb.RequestKind, payload proto.Message) (string, error) {
	pb := cursorpb.Cursor{
		Request: kind,
		Created: timestamppb.New(clock.Now(ctx)),
	}

	switch kind {
	case cursorpb.RequestKind_LIST_BOT_EVENTS, cursorpb.RequestKind_LIST_BOT_TASKS:
		cur, ok := payload.(*cursorpb.OpaqueCursor)
		if !ok {
			panic(fmt.Sprintf("%v: expecting OpaqueCursor, but got %T", kind, payload))
		}
		pb.Payload = &cursorpb.Cursor_OpaqueCursor{OpaqueCursor: cur}
	case cursorpb.RequestKind_LIST_BOTS:
		cur, ok := payload.(*cursorpb.BotsCursor)
		if !ok {
			panic(fmt.Sprintf("%v: expecting BotsCursor, but got %T", kind, payload))
		}
		pb.Payload = &cursorpb.Cursor_BotsCursor{BotsCursor: cur}
	case cursorpb.RequestKind_LIST_TASKS, cursorpb.RequestKind_LIST_TASK_REQUESTS, cursorpb.RequestKind_CANCEL_TASKS:
		cur, ok := payload.(*cursorpb.TasksCursor)
		if !ok {
			panic(fmt.Sprintf("%v: expecting TasksCursor, but got %T", kind, payload))
		}
		pb.Payload = &cursorpb.Cursor_TasksCursor{TasksCursor: cur}
	default:
		panic(fmt.Sprintf("unexpected RequestKind %v", kind))
	}

	blob, err := proto.Marshal(&pb)
	if err != nil {
		logging.Errorf(ctx, "Failed to marshal cursor proto: %s", err)
		return "", cursorEncodeErr
	}

	out, err := secrets.URLSafeEncrypt(ctx, blob, []byte(cursorAEADContext))
	switch {
	case errors.Is(err, secrets.ErrNoPrimaryAEAD):
		logging.Errorf(ctx, "No AEAD key in the context")
		return "", noKeyErr
	case err != nil:
		logging.Errorf(ctx, "Failed to encrypt the cursor: %s", err)
		return "", cursorEncodeErr
	}

	return out, nil
}

// Decode base64-decodes, decrypts and deserializes a cursor.
//
// `T` is expected to have a type matching `kind` (see RequestKind enum in
// cursor.proto), e.g. if `kind` is LIST_BOTS, `T` must be BotsCursor. Panics on
// violations.
//
// Returns gRPC errors with an appropriate code. Such errors can be returned to
// the user as are. Extra details are logged.
func Decode[T any, TP interface {
	*T
	proto.Message
}](ctx context.Context, cursor string, kind cursorpb.RequestKind) (TP, error) {
	blob, err := secrets.URLSafeDecrypt(ctx, cursor, []byte(cursorAEADContext))
	switch {
	case errors.Is(err, secrets.ErrNoPrimaryAEAD):
		logging.Errorf(ctx, "No AEAD key in the context")
		return nil, noKeyErr
	case err != nil:
		logging.Warningf(ctx, "Failed to decrypt the cursor: %s", err)
		return nil, cursorDecodeErr
	}

	var pb cursorpb.Cursor
	if err := proto.Unmarshal(blob, &pb); err != nil {
		logging.Errorf(ctx, "Failed to unmarshal the cursor: %s", err)
		return nil, cursorDecodeErr
	}

	if pb.Request != kind {
		logging.Warningf(ctx, "Expecting cursor of kind %v, but got %v", kind, pb.Request)
		return nil, cursorDecodeErr
	}

	var payload any
	switch kind {
	case cursorpb.RequestKind_LIST_BOT_EVENTS, cursorpb.RequestKind_LIST_BOT_TASKS:
		cur := pb.GetOpaqueCursor()
		if cur == nil {
			logging.Errorf(ctx, "%v: OpaqueCursor is unexpectedly missing")
			return nil, cursorDecodeErr
		}
		payload = cur
	case cursorpb.RequestKind_LIST_BOTS:
		cur := pb.GetBotsCursor()
		if cur == nil {
			logging.Errorf(ctx, "%v: BotsCursor is unexpectedly missing")
			return nil, cursorDecodeErr
		}
		payload = cur
	case cursorpb.RequestKind_LIST_TASKS, cursorpb.RequestKind_LIST_TASK_REQUESTS, cursorpb.RequestKind_CANCEL_TASKS:
		cur := pb.GetTasksCursor()
		if cur == nil {
			logging.Errorf(ctx, "%v: TasksCursor is unexpectedly missing")
			return nil, cursorDecodeErr
		}
		payload = cur
	default:
		panic(fmt.Sprintf("unexpected RequestKind %v", kind))
	}

	// Here `payload` type is already matching `kind`. Per Decode contract, `TP`
	// should also match `kind`, thus `payload` should have type `TP`. If not, it
	// is a programming error and we should panic.
	out, ok := payload.(TP)
	if !ok {
		var zero T
		panic(fmt.Sprintf("%v: expecting %T, but got %T", kind, payload, &zero))
	}

	return out, nil
}

// IsValidCursor returns true if `cursor` looks like a Swarming cursor string.
//
// Returns an error if the check can't be performed (e.g. there's no decryption
// key in the context).
func IsValidCursor(ctx context.Context, cursor string) (bool, error) {
	switch _, err := secrets.URLSafeDecrypt(ctx, cursor, []byte(cursorAEADContext)); {
	case errors.Is(err, secrets.ErrNoPrimaryAEAD):
		logging.Errorf(ctx, "No AEAD key in the context")
		return false, noKeyErr
	case err != nil:
		// Some non-decryptable garbage, probably Python cursor.
		return false, nil
	default:
		// The string was encrypted by us with the correct AEAD context. Since this
		// is an authenticated encryption scheme, it 100% means this string was
		// produced by Encode(...) above, and it is thus a valid cursor string.
		return true, nil
	}
}
