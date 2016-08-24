// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/iotools"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/common/tsmon/types"
)

var (
	// publicSubName is the name of the pubsub subscription that milo is expecting.
	// TODO(hinoka): This should be read from luci-config.
	publicSubName   = "projects/luci-milo/subscriptions/buildbot-public"
	internalSubName = "projects/luci-milo/subscriptions/buildbot-private"

	// Metrics
	buildCounter = metric.NewCounter(
		"luci/milo/buildbot_pubsub/builds",
		"The number of buildbot builds received by Milo from PubSub",
		types.MetricMetadata{},
		field.Bool("internal"),
		field.String("master"),
		field.String("builder"),
		field.Bool("finished"),
		// Status can be one of 3 options.  "New", "Replaced", "Rejected".
		field.String("status"))
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

type buildMasterMsg struct {
	Master *buildbotMaster `json:"master"`
	Builds []buildbotBuild `json:"builds"`
}

// buildbotMasterEntry is a container for a marshaled and packed buildbot
// master json.
type buildbotMasterEntry struct {
	// Name of the buildbot master.
	Name string `gae:"$id"`
	// Internal
	Internal bool
	// Data is the json serialzed and gzipped blob of the master data.
	Data []byte `gae:",noindex"`
	// Modified is when this entry was last modified.
	Modified time.Time
}

func putDSMasterJSON(
	c context.Context, master *buildbotMaster, internal bool) error {
	entry := buildbotMasterEntry{
		Name:     master.Name,
		Internal: internal,
		Modified: clock.Now(c).UTC(),
	}
	gzbs := bytes.Buffer{}
	gsw := gzip.NewWriter(&gzbs)
	cw := iotools.CountingWriter{Writer: gsw}
	e := json.NewEncoder(&cw)
	if err := e.Encode(master); err != nil {
		return err
	}
	gsw.Close()
	entry.Data = gzbs.Bytes()
	log.Debugf(c, "Length of json data: %d", cw.Count)
	log.Debugf(c, "Length of gzipped data: %d", len(entry.Data))
	return datastore.Get(c).Put(&entry)
}

// GetData returns the expanded form of Data (decoded from base64).
func (m *pubSubSubscription) GetData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(m.Message.Data)
}

// unmarshal a gzipped byte stream into a list of buildbot builds and masters.
func unmarshal(
	c context.Context, msg []byte) ([]buildbotBuild, *buildbotMaster, error) {
	bm := buildMasterMsg{}
	if len(msg) == 0 {
		return bm.Builds, bm.Master, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(msg))
	if err != nil {
		log.WithError(err).Errorf(c, "gzip decompression error")
		return nil, nil, err
	}
	defer reader.Close()
	d := json.NewDecoder(reader)
	if err = d.Decode(&bm); err != nil {
		log.WithError(err).Errorf(c, "could not unmarshal message")
		return nil, nil, err
	}
	// Extract the builds out of master and append it onto builds.
	for _, slave := range bm.Master.Slaves {
		if slave.RunningbuildsMap == nil {
			slave.RunningbuildsMap = map[string][]int{}
		}
		for _, build := range slave.Runningbuilds {
			build.Master = bm.Master.Name
			bm.Builds = append(bm.Builds, build)
			slave.RunningbuildsMap[build.Buildername] = append(
				slave.RunningbuildsMap[build.Buildername], build.Number)
		}
		slave.Runningbuilds = nil
	}
	return bm.Builds, bm.Master, nil
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	c, h, r := ctx.Context, ctx.Writer, ctx.Request

	msg := pubSubSubscription{}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		log.WithError(err).Errorf(
			c, "Could not decode message.  %s", err)
		h.WriteHeader(200) // This is a hard failure, we don't want PubSub to retry.
		return
	}
	internal := true
	switch msg.Subscription {
	case publicSubName:
		internal = false
	case internalSubName:
		// internal = true, but that's already set.
	default:
		log.Errorf(
			c, "Subscription name %s does not match %s or %s",
			msg.Subscription, publicSubName, internalSubName)
		h.WriteHeader(200)
		return
	}
	bbMsg, err := msg.GetData()
	if err != nil {
		log.WithError(err).Errorf(c, "Could not base64 decode message %s", err)
		h.WriteHeader(200)
		return
	}
	builds, master, err := unmarshal(c, bbMsg)
	if err != nil {
		log.WithError(err).Errorf(c, "Could not unmarshal message %s", err)
		h.WriteHeader(200)
		return
	}
	log.Infof(c, "There are %d builds and master %s", len(builds), master.Name)
	ds := datastore.Get(c)
	// Do not use PutMulti because we might hit the 1MB limit.
	for _, build := range builds {
		log.Debugf(
			c, "Checking for build %s/%s/%d", build.Master, build.Buildername, build.Number)
		if build.Master == "" {
			log.Errorf(c, "Invalid message, missing master name")
			h.WriteHeader(200)
			return
		}
		existingBuild := &buildbotBuild{
			Master:      build.Master,
			Buildername: build.Buildername,
			Number:      build.Number,
		}
		buildExists := false
		if err := ds.Get(existingBuild); err == nil {
			if existingBuild.Finished {
				// Never replace a completed build.
				buildCounter.Add(
					c, 1, false, build.Master, build.Buildername, false, "Rejected")
				log.Debugf(
					c, "Found build %s/%s/%d and it's finished, skipping", build.Master, build.Buildername, build.Number)
				continue
			}
			buildExists = true
		}
		// Also set the finished and internal bit.
		build.Finished = false
		if len(build.Times) == 2 && build.Times[1] != nil {
			build.Finished = true
		}
		build.Internal = internal
		err = ds.Put(&build)
		if err != nil {
			switch err {
			case errTooBig:
				// This will never work, we don't want PubSub to retry.
				log.WithError(err).Errorf(
					c, "Could not save build to datastore, failing permanently")
				h.WriteHeader(200)
			default:
				// This is transient, we do want PubSub to retry.
				log.WithError(err).Errorf(c, "Could not save build in datastore")
				h.WriteHeader(500)
			}
			return
		}
		log.Debugf(
			c, "Saved build %s/%s/%d, it is %s",
			build.Master, build.Buildername, build.Number, build.Finished)
		if buildExists {
			buildCounter.Add(
				c, 1, false, build.Master, build.Buildername, build.Finished, "Replaced")
		} else {
			buildCounter.Add(
				c, 1, false, build.Master, build.Buildername, build.Finished, "New")
		}

	}
	if master != nil {
		err = putDSMasterJSON(c, master, internal)
		if err != nil {
			log.WithError(err).Errorf(
				c, "Could not save master in datastore %s", err)
			// This is transient, we do want PubSub to retry.
			h.WriteHeader(500)
			return
		}
	}
	h.WriteHeader(200)
}
