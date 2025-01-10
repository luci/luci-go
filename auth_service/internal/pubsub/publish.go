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

package pubsub

import (
	"context"
	"encoding/base64"
	"fmt"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// PublishAuthDBRevision notifies subscribers there's another revision
// of the AuthDB available.
//
// Publishing is skipped if:
// - the AuthDBRevision is invalid; or
// - the app server is a local development server.
func PublishAuthDBRevision(ctx context.Context, rev *protocol.AuthDBRevision) error {
	if rev == nil {
		return fmt.Errorf("invalid AuthDBRevision - aborting")
	}

	// Skip publishing if this is a local development server.
	if info.IsDevAppServer(ctx) {
		logging.Debugf(ctx, "on dev server - skipping PubSub publishing for AuthDB rev: %d",
			rev.AuthDbRev)
		return nil
	}

	return publish(ctx, rev)
}

// publish constructs the pubsub.Message.
func publish(ctx context.Context, rev *protocol.AuthDBRevision) (retErr error) {
	pushReq := &protocol.ReplicationPushRequest{
		Revision: rev,
	}
	data, err := proto.Marshal(pushReq)
	if err != nil {
		return errors.Annotate(err, "error marshalling ReplicationPushRequest for PubSub message").Err()
	}
	signer := auth.GetSigner(ctx)
	if signer == nil {
		return errors.New("no signer - aborting")
	}
	keyName, sig, err := signer.SignBytes(ctx, data)
	if err != nil {
		return errors.Annotate(err, "error signing payload").Err()
	}

	// Construct the PubSub message to be published.
	msg := &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"X-AuthDB-SigKey-v1": keyName,
			"X-AuthDB-SigVal-v1": base64.StdEncoding.EncodeToString(sig),
		},
	}

	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating PubSub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	if err := client.Publish(ctx, msg); err != nil {
		return err
	}

	return nil
}
