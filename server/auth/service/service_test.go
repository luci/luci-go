// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/service/protocol"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/auth/signing/signingtest"
	"github.com/luci/luci-go/server/proccache"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPubSubWorkflow(t *testing.T) {
	Convey("PubSub pull workflow works", t, func(c C) {
		ctx := proccache.Use(context.Background(), &proccache.Cache{})

		fakeSigner := signingtest.NewSigner(0)
		certs, _ := fakeSigner.Certificates(ctx)
		certsJSON, _ := json.Marshal(certs)

		// Expected calls.
		calls := []struct {
			Method   string
			URL      string
			Code     int
			Response string
		}{
			// Probing for existing subscription -> tell it is not found.
			{
				"GET",
				"/pubsub/projects/p1/subscriptions/sub",
				404,
				`{"error": {"code": 404}}`,
			},
			// Authorizing access to PubSub topic.
			{
				"POST",
				"/auth_service/api/v1/authdb/subscription/authorization",
				200,
				`{"topic": "projects/p2/topics/topic"}`,
			},
			// Creating the subscription.
			{
				"PUT",
				"/pubsub/projects/p1/subscriptions/sub",
				200,
				"",
			},
			// Probing for existing subscription again.
			{
				"GET",
				"/pubsub/projects/p1/subscriptions/sub",
				200,
				`{"pushConfig": {"pushEndpoint": "http://blah"}}`,
			},
			// Changing push URL.
			{
				"POST",
				"/pubsub/projects/p1/subscriptions/sub:modifyPushConfig",
				200,
				"",
			},
			// Pulling messages from it, all bad.
			{
				"POST",
				"/pubsub/projects/p1/subscriptions/sub:pull",
				200,
				`{"receivedMessages": [
					{
						"ackId": "ack1",
						"message": {"data": "broken"}
					}
				]}`,
			},
			// Fetching certificates from auth service to authenticate messages.
			{
				"GET",
				"/auth/api/v1/server/certificates",
				200,
				string(certsJSON),
			},
			// Bad messages are removed from the queue by ack.
			{
				"POST",
				"/pubsub/projects/p1/subscriptions/sub:acknowledge",
				200,
				"",
			},
			// Pulling messages from the subscription, again.
			{
				"POST",
				"/pubsub/projects/p1/subscriptions/sub:pull",
				200,
				fmt.Sprintf(`{"receivedMessages": [
					{
						"ackId": "ack1",
						"message": {"data": "broken"}
					},
					%s,
					%s
				]}`, fakePubSubMessage(ctx, "ack2", 122, fakeSigner), fakePubSubMessage(ctx, "ack2", 123, fakeSigner)),
			},
			// Acknowledging messages.
			{
				"POST",
				"/pubsub/projects/p1/subscriptions/sub:acknowledge",
				200,
				"",
			},
		}
		counter := 0

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if counter >= len(calls) {
				c.So(fmt.Sprintf("%s %s is unexpected", r.Method, r.URL.Path), ShouldBeNil)
			}
			call := &calls[counter]
			c.So(r.Method, ShouldEqual, call.Method)
			c.So(r.URL.Path, ShouldEqual, call.URL)
			w.WriteHeader(call.Code)
			if call.Response != "" {
				w.Write([]byte(call.Response))
			}
			counter++
		}))

		// Register.
		srv := AuthService{
			URL:           ts.URL,
			pubSubURLRoot: ts.URL + "/pubsub/",
		}
		So(srv.EnsureSubscription(ctx, "projects/p1/subscriptions/sub", "http://blah"), ShouldBeNil)

		// Reregister with no push url. For code coverage.
		So(srv.EnsureSubscription(ctx, "projects/p1/subscriptions/sub", ""), ShouldBeNil)

		// First pull. No valid messages.
		notify, err := srv.PullPubSub(ctx, "projects/p1/subscriptions/sub")
		So(err, ShouldBeNil)
		So(notify, ShouldBeNil)

		// Second pull. Have something.
		notify, err = srv.PullPubSub(ctx, "projects/p1/subscriptions/sub")
		So(err, ShouldBeNil)
		So(notify, ShouldNotBeNil)
		So(notify.Revision, ShouldEqual, 123)

		// Ack.
		So(notify.Acknowledge(ctx), ShouldBeNil)

		// Code coverage.
		notify, err = srv.ProcessPubSubPush(ctx, []byte(fakePubSubMessage(ctx, "", 456, fakeSigner)))
		So(err, ShouldBeNil)
		So(notify, ShouldNotBeNil)
		So(notify.Revision, ShouldEqual, 456)
	})
}

func fakePubSubMessage(c context.Context, ackID string, rev int64, signer signing.Signer) string {
	primaryID := "primaryId"
	ts := int64(1000)
	msg := protocol.ChangeNotification{
		Revision: &protocol.AuthDBRevision{
			AuthDbRev:  &rev,
			PrimaryId:  &primaryID,
			ModifiedTs: &ts,
		},
	}
	blob, _ := proto.Marshal(&msg)
	key, sig, _ := signer.SignBytes(c, blob)
	ps := pubSubMessage{
		AckID: ackID,
	}
	ps.Message.Data = base64.StdEncoding.EncodeToString(blob)
	ps.Message.Attributes = map[string]string{
		"X-AuthDB-SigKey-v1": key,
		"X-AuthDB-SigVal-v1": base64.StdEncoding.EncodeToString(sig),
	}
	out, _ := json.Marshal(&ps)
	return string(out)
}
