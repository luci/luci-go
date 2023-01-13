// Copyright 2023 The LUCI Authors.
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

// Package hmactoken implements generation and validation HMAC-tagged Swarming
// tokens.
package hmactoken

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

// Secret can be used to generate and validate HMAC-tagged tokens.
type Secret struct {
	hmacSecret atomic.Value // stores secrets.Secret
}

// NewRotatingSecret creates a new secret given a key name and subscribes to its
// rotations.
func NewRotatingSecret(ctx context.Context, keyName string) (*Secret, error) {
	s := &Secret{}

	// Load the initial value of the key used to HMAC-tag poll tokens.
	key, err := secrets.StoredSecret(ctx, keyName)
	if err != nil {
		return nil, err
	}
	s.hmacSecret.Store(key)

	// Update the cached value whenever the secret rotates.
	err = secrets.AddRotationHandler(ctx, keyName, func(_ context.Context, key secrets.Secret) {
		s.hmacSecret.Store(key)
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}

// NewStaticSecret creates a new secret from a given static secret value.
//
// Mostly for tests.
func NewStaticSecret(secret secrets.Secret) *Secret {
	s := &Secret{}
	s.hmacSecret.Store(secret)
	return s
}

// ValidateToken deserializes a TaggedMessage, checks the HMAC and deserializes
// the payload into `msg`.
func (s *Secret) ValidateToken(tok []byte, msg proto.Message) error {
	// Deserialize the envelope.
	var envelope internalspb.TaggedMessage
	if err := proto.Unmarshal(tok, &envelope); err != nil {
		return errors.Annotate(err, "failed to deserialize TaggedMessage").Err()
	}
	if expected := taggedMessagePayload(msg); envelope.PayloadType != expected {
		return errors.Reason("invalid payload type %v, expecting %v", envelope.PayloadType, expected).Err()
	}

	// Verify the HMAC. It must be produced using any of the secret versions.
	valid := false
	secret := s.hmacSecret.Load().(secrets.Secret)
	for _, key := range secret.Blobs() {
		// See rbe_pb2.TaggedMessage.
		mac := hmac.New(sha256.New, key)
		_, _ = fmt.Fprintf(mac, "%d\n", envelope.PayloadType)
		_, _ = mac.Write(envelope.Payload)
		expected := mac.Sum(nil)
		if hmac.Equal(expected, envelope.HmacSha256) {
			valid = true
			break
		}
	}
	if !valid {
		return errors.Reason("bad token HMAC").Err()
	}

	// The payload can be trusted.
	if err := proto.Unmarshal(envelope.Payload, msg); err != nil {
		return errors.Annotate(err, "failed to deserialize token payload").Err()
	}
	return nil
}

// GenerateToken wraps `msg` into a serialized TaggedMessage.
//
// The produced token can be validated and deserialized with validateToken.
func (s *Secret) GenerateToken(msg proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize the token payload").Err()
	}

	// The future token, but without HMAC yet.
	envelope := internalspb.TaggedMessage{
		PayloadType: taggedMessagePayload(msg),
		Payload:     payload,
	}

	// See rbe_pb2.TaggedMessage.
	secret := s.hmacSecret.Load().(secrets.Secret).Active
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%d\n", envelope.PayloadType)
	_, _ = mac.Write(envelope.Payload)
	envelope.HmacSha256 = mac.Sum(nil)

	token, err := proto.Marshal(&envelope)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize the token").Err()
	}
	return token, nil
}

// taggedMessagePayload examines the type of msg and returns the corresponding
// enum variant.
//
// Panics if it is a completely unexpected message.
func taggedMessagePayload(msg proto.Message) internalspb.TaggedMessage_PayloadType {
	switch msg.(type) {
	case *internalspb.PollState:
		return internalspb.TaggedMessage_POLL_STATE
	case *internalspb.BotSession:
		return internalspb.TaggedMessage_BOT_SESSION
	default:
		panic(fmt.Sprintf("unexpected message type %T", msg))
	}
}
