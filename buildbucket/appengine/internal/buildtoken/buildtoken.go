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
	"encoding/base64"

	"github.com/google/tink/go/subtle/random"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// Sanity length limitation for build tokens to allow us to quickly reject
	// potentially abusive inputs.
	buildTokenMaxLength = 200
)

var BuildTokenInOldFormat = errors.BoolTag{Key: errors.NewTagKey("build token in the old format")}

// GenerateToken generates base64 encoded byte string token for a build.
// In the future, it will be replaced by a self-verifiable token.
func GenerateToken(buildID int64) (string, error) {
	tkBody := &pb.TokenBody{
		BuildId: buildID,
		Purpose: pb.TokenBody_BUILD,
		State:   random.GetRandomBytes(16),
	}

	tkBytes, err := proto.Marshal(tkBody)
	if err != nil {
		return "", err
	}
	tkEnvelop := &pb.TokenEnvelope{
		Version: pb.TokenEnvelope_UNENCRYPTED_PASSWORD_LIKE,
		Payload: tkBytes,
	}
	tkeBytes, err := proto.Marshal(tkEnvelop)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tkeBytes)
	return token, nil
}

// ParseToTokenBody deserializes the build token and returns the token body.
func ParseToTokenBody(bldTok string) (*pb.TokenBody, error) {
	if len(bldTok) > buildTokenMaxLength {
		return nil, errors.Reason("build token %s is too long", bldTok).Err()
	}
	tokBytes, err := base64.RawURLEncoding.DecodeString(bldTok)
	if err != nil {
		return nil, errors.Reason("error decoding token").Err()
	}

	msg := &pb.TokenEnvelope{}
	if err := proto.Unmarshal(tokBytes, msg); err != nil {
		return nil, errors.Reason("error unmarshalling token").Tag(BuildTokenInOldFormat).Err()
	}

	if msg.Version != pb.TokenEnvelope_UNENCRYPTED_PASSWORD_LIKE {
		return nil, errors.Reason("token with version %d is not supported", msg.Version).Err()
	}

	body := &pb.TokenBody{}
	if err := proto.Unmarshal(msg.Payload, body); err != nil {
		return nil, errors.Reason("error unmarshalling token payload").Err()
	}
	return body, nil
}
