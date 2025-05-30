// Copyright 2022 The LUCI Authors.
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

// Package buildtoken provide related functions for dealing with build tokens.
package buildtoken

import (
	"context"
	"encoding/base64"

	"github.com/google/tink/go/subtle/random"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/secrets"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// Sanity length limitation for build tokens to allow us to quickly reject
	// potentially abusive inputs.
	buildTokenMaxLength = 200
)

// additionalData gives additional context to an encrypted secret, to prevent
// the cyphertext from being used in contexts other than this buildtoken
// package.
var additionalData = []byte("buildtoken")

// GenerateToken generates base64 encoded byte string token for a build.
func GenerateToken(ctx context.Context, buildID int64, purpose pb.TokenBody_Purpose) (string, error) {
	return generateEncryptedToken(ctx, buildID, purpose)
}

func generateEncryptedToken(ctx context.Context, buildID int64, purpose pb.TokenBody_Purpose) (string, error) {
	tkBody := &pb.TokenBody{
		BuildId: buildID,
		Purpose: purpose,
		State:   random.GetRandomBytes(16),
	}

	tkBytes, err := proto.Marshal(tkBody)
	if err != nil {
		return "", err
	}
	encBytes, err := secrets.Encrypt(ctx, tkBytes, additionalData)
	if err != nil {
		return "", err
	}
	tkEnvelop := &pb.TokenEnvelope{
		Version: pb.TokenEnvelope_ENCRYPTED,
		Payload: encBytes,
	}
	tkeBytes, err := proto.Marshal(tkEnvelop)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tkeBytes)
	return token, nil
}

// ErrBadToken is the only error returned by ParseToTokenBody.
//
// This includes a codes.Unauthenticated tag.
var ErrBadToken = grpcutil.UnauthenticatedTag.Apply(

	// ParseToTokenBody deserializes the build token and returns the token body.
	//
	// buildID will be asserted to match the token's contents.
	// If buildID is 0, this will skip the buildID check.
	//
	// Additionally, the token contents must match one of the values provided in
	// `purposes`. If `purposes` is empty, a token of any purpose will be returned
	// without error.
	//
	// All parsing errors are logged and this function returns ErrBadToken which is
	// tagged with codes.Unauthenticated.
	errors.New("invalid token"))

func ParseToTokenBody(ctx context.Context, bldTok string, buildID int64, purposes ...pb.TokenBody_Purpose) (*pb.TokenBody, error) {
	tok, err := parseToTokenBodyImpl(ctx, bldTok, buildID, purposes...)
	if err != nil {
		logging.Warningf(ctx, "ParseToTokenBody ERROR: %s", err)
		return nil, ErrBadToken
	}
	return tok, nil
}

func parseToTokenBodyImpl(ctx context.Context, bldTok string, buildID int64, purposes ...pb.TokenBody_Purpose) (*pb.TokenBody, error) {
	if len(bldTok) > buildTokenMaxLength {
		return nil, errors.Fmt("build token is too long: %d > %d", len(bldTok), buildTokenMaxLength)
	}
	tokBytes, err := base64.RawURLEncoding.DecodeString(bldTok)
	if err != nil {
		return nil, errors.Fmt("error decoding token: %w", err)
	}

	msg := &pb.TokenEnvelope{}
	if err = proto.Unmarshal(tokBytes, msg); err != nil {
		return nil, errors.Fmt("error unmarshalling token: %w", err)
	}

	var payload []byte

	switch msg.Version {
	case pb.TokenEnvelope_ENCRYPTED:
		if payload, err = secrets.Decrypt(ctx, msg.Payload, additionalData); err != nil {
			return nil, errors.Fmt("error decrypting token: %w", err)
		}

	default:
		return nil, errors.Fmt("token with version %d is not supported", msg.Version)
	}

	tb := &pb.TokenBody{}
	if err = proto.Unmarshal(payload, tb); err != nil {
		return nil, errors.Fmt("error unmarshalling token payload: %w", err)
	}

	if buildID != 0 && buildID != tb.BuildId {
		return nil, errors.Fmt("token is for build %d, but expected %d", tb.BuildId, buildID)
	}

	if len(purposes) > 0 {
		ok := false
		for _, purpose := range purposes {
			if purpose == tb.Purpose {
				ok = true
				break
			}
		}
		if !ok {
			return nil, errors.Fmt("token is for purpose %s, but expected %s", tb.Purpose, purposes)
		}
	}

	return tb, nil
}
