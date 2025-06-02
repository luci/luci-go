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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/internal"
	"go.chromium.org/luci/server/router"
)

var (
	callsCounter = metric.NewCounter(
		"pubsub/server/calls",
		"Count of handled pubsub message pushes",
		nil,
		field.String("id"),     // pubsub handler ID
		field.String("result"), // OK | ignore | transient | fatal | panic | no_handler | auth
	)

	callsDurationMS = metric.NewCumulativeDistribution(
		"pubsub/server/duration",
		"Duration of handling of recognized handlers",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("id"),     // pubsub handler ID
		field.String("result"), // OK | ignore | transient | fatal | panic
	)
)

var (
	// Ignore is an error tag used to indicate that the handler wants the
	// pub/sub message to be dropped as it is not useful (e.g. represents
	// an event that the service does not ingest, such a build state changing
	// to running when the service is only interested in build completions,
	// or a replay of a previous accepted message).
	//
	// This results in the pub/sub push handler returning status 204
	// (No Content) as opposed to status 200. This is particularly
	// useful for allowing SLOs to be defined over useful messages only.
	Ignore = errtag.Make("the message should be dropped as not useful", true)
)

type Message struct {
	// Data is the PubSub message payload.
	Data []byte
	// Subscription is the full subscription name that pushed the message.
	// Format: projects/myproject/subscriptions/mysubscription.
	Subscription string
	// MessageID is the PubSub message ID.
	MessageID string
	// PublishTime is when the message was published.
	PublishTime time.Time
	// Attributes is PubSub message attributes of the published message.
	// Guaranteed to be non-nil.
	Attributes map[string]string
	// Query is the query part of the HTTP request string.
	// Guaranteed to be non-nil.
	Query url.Values
}

// Handler is called to handle a pub/sub message.
//
// Transient errors are transformed into HTTP 500 replies to Cloud Pub/Sub,
// which may trigger a retry based on the pub/sub subscription retry policy.
// Returning a non-transient error results in a error-level logging message
// and HTTP 202 reply, which does not trigger a retry.
type Handler func(ctx context.Context, message Message) error

// JSONPB wraps a handler by deserializing messages as JSONPB protobufs
// before passing them to the handler.
func JSONPB[T any, TP interface {
	*T
	proto.Message
}](handler func(context.Context, Message, TP) error) Handler {
	return func(ctx context.Context, message Message) error {
		var msg TP = new(T)
		opts := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := opts.Unmarshal(message.Data, msg); err != nil {
			return errors.Fmt("parsing PubSub message as jsonpb proto: %w", err)
		}
		return handler(ctx, message, msg)
	}
}

// WirePB wraps a handler by deserializing messages as protobufs in wire
// encoding before passing them to the handler.
func WirePB[T any, TP interface {
	*T
	proto.Message
}](handler func(context.Context, Message, TP) error) Handler {
	return func(ctx context.Context, message Message) error {
		var msg TP = new(T)
		if err := proto.Unmarshal(message.Data, msg); err != nil {
			return errors.Fmt("parsing PubSub message as wirepb proto: %w", err)
		}
		return handler(ctx, message, msg)
	}
}

// Dispatcher routes requests from Cloud Pub/Sub to registered handlers.
type Dispatcher struct {
	// AuthorizedCallers is a list of service accounts Cloud Pub/Sub may use to
	// call pub/sub HTTP endpoints.
	//
	// See https://cloud.google.com/pubsub/docs/authenticate-push-subscriptions for details.
	AuthorizedCallers []string

	// DisableAuth can be used to disable authentication on HTTP endpoints.
	//
	// This is useful when running in development mode on localhost or in tests.
	DisableAuth bool

	m sync.RWMutex
	h map[string]Handler
}

// handlerIDRe is used to validate handler IDs.
var handlerIDRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]{1,100}$`)

// RegisterHandler registers a callback called to handle a pubsub message.
//
// The handler can be invoked via POST requests to "<serving-prefix>/<id>",
// (usually "/internal/pubsub/<id>"). This is the push endpoint that should be used
// when configuring Cloud Pub/Sub subscriptions. The Pub/Sub push subscription
// must be configured to send wrapped messages.
//
// The ID must match `[a-zA-Z0-9_\-.]{1,100}`. Panics otherwise. Panics if a
// handler with such ID is already registered.
func (d *Dispatcher) RegisterHandler(id string, h Handler) {
	if !handlerIDRe.MatchString(id) {
		panic(fmt.Sprintf("bad pubsub handler ID %q", id))
	}
	d.m.Lock()
	defer d.m.Unlock()
	if d.h == nil {
		d.h = make(map[string]Handler, 1)
	}
	if _, ok := d.h[id]; ok {
		panic(fmt.Sprintf("pubsub handler with ID %q is already registered", id))
	}
	d.h[id] = h
}

// InstallPubSubRoutes installs routes that handle requests from Cloud Pub/Sub.
func (d *Dispatcher) InstallPubSubRoutes(r *router.Router, prefix string) {
	if prefix == "" {
		prefix = "/internal/pubsub/"
	} else if !strings.HasPrefix(prefix, "/") {
		panic("the prefix should start with /")
	}

	route := strings.TrimRight(prefix, "/") + "/*handler"
	handlerID := func(c *router.Context) string {
		return strings.TrimPrefix(c.Params.ByName("handler"), "/")
	}

	var mw router.MiddlewareChain
	if !d.DisableAuth {
		header := ""
		mw = internal.CloudAuthMiddleware(d.AuthorizedCallers, header,
			func(c *router.Context) {
				callsCounter.Add(c.Request.Context(), 1, handlerID(c), "auth")
			},
		)
	}

	r.POST(route, mw, func(c *router.Context) {
		id := handlerID(c)

		if err := d.executeHandlerByID(c.Request.Context(), id, c); err != nil {
			if transient.Tag.In(err) {
				err = errors.Fmt("transient error in pubsub handler %q: %w", id, err)
				errors.Log(c.Request.Context(), err)
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError /* 500 */)
			} else if Ignore.In(err) {
				http.Error(c.Writer, "", http.StatusNoContent /* 204 */)
			} else {
				err = errors.Fmt("fatal error in pubsub handler %q: %w", id, err)
				errors.Log(c.Request.Context(), err)
				http.Error(c.Writer, err.Error(), http.StatusAccepted /* 202 */)
			}
		} else {
			c.Writer.Write([]byte("OK"))
		}
	})
}

// handlerIDs returns a sorted list of registered handler IDs.
func (d *Dispatcher) handlerIDs() []string {
	d.m.RLock()
	defer d.m.RUnlock()
	ids := make([]string, 0, len(d.h))
	for id := range d.h {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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

// readMessageWrapper reads the contents of a POST request for
// a Cloud Pub/Sub push subscription using wrapped messages.
func readMessageWrapper(c *router.Context) (pushRequestBody, error) {
	bodyBlob, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return pushRequestBody{}, transient.Tag.Apply(errors.Fmt("reading request body: %w", err))
	}
	// Deserialize the push message wrapper.
	var body pushRequestBody
	if err := json.Unmarshal(bodyBlob, &body); err != nil {
		return pushRequestBody{}, errors.Fmt("bad push request body: %w", err)
	}
	if body.Subscription == "" {
		return pushRequestBody{}, errors.New("bad request body, missing field 'subscription'; did you configure your pub/sub subscription to use wrapped messages?")
	}
	return body, nil
}

// executeHandlerByID executes a registered pub/sub handler.
func (d *Dispatcher) executeHandlerByID(ctx context.Context, id string, c *router.Context) error {
	d.m.RLock()
	h := d.h[id]
	d.m.RUnlock()
	if h == nil {
		callsCounter.Add(ctx, 1, id, "no_handler")
		return errors.Fmt("no pubsub handler with ID %q is registered", id)
	}

	start := clock.Now(ctx)
	result := "panic"
	defer func() {
		callsCounter.Add(ctx, 1, id, result)
		callsDurationMS.Add(ctx, float64(clock.Since(ctx, start).Milliseconds()), id, result)
	}()

	// Parse the push message wrapper.
	msg, err := readMessageWrapper(c)
	if err != nil {
		if transient.Tag.In(err) {
			result = "transient"
		} else {
			result = "fatal"
		}
		return errors.Fmt("reading pub/sub message wrapper: %w", err)
	}

	message := Message{
		Data:         msg.Message.Data,
		Subscription: msg.Subscription,
		MessageID:    msg.Message.MessageID,
		PublishTime:  msg.Message.PublishTime,
		Attributes:   msg.Message.Attributes,
		Query:        c.Request.URL.Query(),
	}
	if message.Attributes == nil {
		message.Attributes = map[string]string{}
	}

	err = h(ctx, message)
	switch {
	case err == nil:
		result = "OK"
	case transient.Tag.In(err):
		result = "transient"
	case Ignore.In(err):
		result = "ignore"
	default:
		result = "fatal"
	}
	return err
}
