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

// Package pubsub allows to install PubSub push handlers.
package pubsub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
)

// HandlerOptions is a configuration of the PubSub push handler route.
type HandlerOptions struct {
	// Route is a HTTP server route to install the handler under.
	Route string

	// PushServiceAccount is a service account email that PubSub uses to
	// authenticate pushes.
	//
	// See https://cloud.google.com/pubsub/docs/authenticate-push-subscriptions.
	PushServiceAccount string
}

// Message is a constraint on a message type for the handler callback.
type Message[T any] interface {
	proto.Message
	*T
}

// Metadata is all the extra information about the incoming pubsub push.
type Metadata struct {
	// Subscription is the full subscription name that pushed the message.
	Subscription string
	// MessageID is the PubSub message ID.
	MessageID string
	// PublishTime is when the message was published
	PublishTime time.Time
	// Attributes is PubSub message attributes of the published message.
	Attributes map[string]string
	// Query is the query part of the HTTP request string.
	Query url.Values
}

// InstallHandler installs a route that processes PubSub push messages.
//
// If the handler callback returns an error tagged with transient.Tag, the
// response will have HTTP 500 code, which will trigger redelivery on the
// message (per PubSub subscription retry policy).
//
// No errors or errors without transient.Tag result in HTTP 2xx replies. PubSub
// will redeliver the message.
func InstallHandler[T any, M Message[T]](
	r *router.Router,
	opts HandlerOptions,
	cb func(ctx context.Context, msg M, md *Metadata) error,
) {
	// Authenticate requests based on OpenID identity tokens.
	oidcMW := router.NewMiddlewareChain(
		auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
			AudienceCheck: openid.AudienceMatchesHost,
		}),
	)

	// Expected authenticated identity of the PubSub service.
	pusherID, err := identity.MakeIdentity("user:" + opts.PushServiceAccount)
	if err != nil {
		panic(err)
	}

	r.POST(opts.Route, oidcMW, func(ctx *router.Context) { handler(ctx, pusherID, cb) })
}

// handler is actual POST request handler extract into a separate function for
// easier testing.
func handler[T any, M Message[T]](
	ctx *router.Context,
	pusherID identity.Identity,
	cb func(ctx context.Context, msg M, md *Metadata) error,
) {
	rctx := ctx.Request.Context()

	if got := auth.CurrentIdentity(rctx); got != pusherID {
		logging.Errorf(rctx, "Expecting ID token of %q, got %q", pusherID, got)
		ctx.Writer.WriteHeader(http.StatusForbidden)
		return
	}

	// Deserialize the push message wrapper.
	var body pushRequestBody
	if err := json.NewDecoder(ctx.Request.Body).Decode(&body); err != nil {
		logging.Errorf(rctx, "Bad push request body: %s", err)
		ctx.Writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Deserialize the message payload.
	var msg T
	if err := proto.Unmarshal(body.Message.Data, M(&msg)); err != nil {
		logging.Errorf(rctx, "Failed to deserialize push message: %s", err)
		ctx.Writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Pass to the handler and process the error.
	err := cb(rctx, &msg, &Metadata{
		MessageID:    body.Message.MessageID,
		Subscription: body.Subscription,
		PublishTime:  body.Message.PublishTime,
		Attributes:   body.Message.Attributes,
		Query:        ctx.Request.URL.Query(),
	})
	switch {
	case err == nil:
		// Success!
		ctx.Writer.WriteHeader(http.StatusOK)
	case transient.Tag.In(err):
		// Transient error, trigger a retry by returning 5xx response.
		logging.Errorf(rctx, "Transient error: %s", err)
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	default:
		// Fatal error, do not trigger a retry by returning 2xx response.
		logging.Errorf(rctx, "Fatal error: %s", err)
		ctx.Writer.WriteHeader(http.StatusAccepted)
	}
}

// pushRequestBody is a JSON body of a messages of a wrapped push subscription.
//
// See https://cloud.google.com/pubsub/docs/push.
type pushRequestBody struct {
	Message struct {
		Attributes  map[string]string `json:"attributes,omitempty"`
		Data        []byte            `json:"data"`
		MessageID   string            `json:"message_id"`
		PublishTime time.Time         `json:"publish_time"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}
