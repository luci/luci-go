// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/luci/gae/service/datastore"
	log "github.com/luci/luci-go/common/logging"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
)

var (
	// subName is the name of the pubsub subscription that milo is expecting.
	// TODO(hinoka): This should be read from luci-config.
	subName = "projects/luci-milo/subscriptions/buildbot-public"
)

type pubSubMessage struct {
	Attributes map[string]string `json:"attributes"`
	Data       string            `json:"data"`
	MessageID  string            `json:"message_id"`
}

type pubSubSubscription struct {
	Message      pubSubMessage `json:"message"`
	Subscription string        `json:"subscription"`
}

// GetData returns the expanded form of Data (decoded from base64).
func (m *pubSubSubscription) GetData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(m.Message.Data)
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(c context.Context, h http.ResponseWriter, r *http.Request, p httprouter.Params) {
	msg := pubSubSubscription{}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		log.Errorf(c, "Could not decode message.  %s", err.Error())
		h.WriteHeader(200) // This is a hard failure, we don't want PubSub to retry.
		return
	}
	if msg.Subscription != subName {
		log.Errorf(c, "Subscription name %s does not match %s", msg.Subscription, subName)
		h.WriteHeader(200)
		return
	}
	bbMsg, err := msg.GetData()
	if err != nil {
		log.Errorf(c, "Could not decode message %s", err.Error())
		h.WriteHeader(200)
		return
	}
	builds := []buildbotBuild{}
	err = json.Unmarshal(bbMsg, &builds)
	if err != nil {
		log.Errorf(c, "Could not unmarshal message %s", err.Error())
		h.WriteHeader(200)
		return
	}
	log.Debugf(c, "There are %d builds", len(builds))
	ds := datastore.Get(c)
	// Do not use PutMulti because we might hit the 1MB limit.
	for _, build := range builds {
		existingBuild := &buildbotBuild{
			Master:      build.Master,
			Buildername: build.Buildername,
			Number:      build.Number,
		}
		if err := ds.Get(existingBuild); err == nil {
			if existingBuild.Times[1] != nil && build.Times[1] == nil {
				// Never replace a completed build with a pending build.
				continue
			}
		}
		if build.Times[1] == nil {
			log.Debugf(c, "Found a pending build")
		}
		err = ds.Put(&build)
		if err != nil {
			log.Errorf(c, "Could not save in datastore %s", err.Error())
			// This is transient, we do want PubSub to retry.
			h.WriteHeader(500)
			return
		}
	}
	h.WriteHeader(200)
}
