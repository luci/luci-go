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

// Package botsession implements marshaling of Bot Session protos.
package botsession

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

// Expiry is how long a new Swarming session token will last.
const Expiry = time.Hour

// cryptoCtx is used whe signing and checking the token as a cryptographic
// context (to make sure produced token can't be incorrectly used in other
// protocols that use the same secret key).
const cryptoCtx = "swarming.Session"

// Marshal serializes and signs a bot session proto.
func Marshal(s *internalspb.Session, secret *hmactoken.Secret) ([]byte, error) {
	blob, err := proto.Marshal(s)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&internalspb.SessionToken{
		Kind: &internalspb.SessionToken_HmacTagged_{
			HmacTagged: &internalspb.SessionToken_HmacTagged{
				Session:    blob,
				HmacSha256: secret.Tag([]byte(cryptoCtx), blob),
			},
		},
	})
}

// Unmarshal checks the signature and deserializes a bot session.
//
// Doesn't check expiration time or validity of any Session fields.
func Unmarshal(tok []byte, secret *hmactoken.Secret) (*internalspb.Session, error) {
	var wrap internalspb.SessionToken
	if err := proto.Unmarshal(tok, &wrap); err != nil {
		return nil, errors.Annotate(err, "unmarshaling SessionToken").Err()
	}

	var blob []byte
	switch val := wrap.Kind.(type) {
	case *internalspb.SessionToken_HmacTagged_:
		if !secret.Verify([]byte(cryptoCtx), val.HmacTagged.Session, val.HmacTagged.HmacSha256) {
			return nil, errors.Reason("bad session token MAC").Err()
		}
		blob = val.HmacTagged.Session
	default:
		return nil, errors.Reason("unsupported session token format").Err()
	}

	s := &internalspb.Session{}
	if err := proto.Unmarshal(blob, s); err != nil {
		return nil, errors.Annotate(err, "unmarshaling Session").Err()
	}
	return s, nil
}

// FormatForDebug formats the session proto for the debug log.
func FormatForDebug(s *internalspb.Session) string {
	blob, err := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(s)
	if err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	return string(blob)
}

// DebugInfo generates new DebugInfo proto identifying the current request.
func DebugInfo(ctx context.Context, backendVer string) *internalspb.DebugInfo {
	return &internalspb.DebugInfo{
		Created:         timestamppb.New(clock.Now(ctx)),
		SwarmingVersion: backendVer,
		RequestId:       trace.SpanContextFromContext(ctx).TraceID().String(),
	}
}
