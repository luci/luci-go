// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/iotools"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/router"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
)

var (
	// Metrics
	buildCounter = metric.NewCounter(
		"luci/milo/buildbot_pubsub/builds",
		"The number of buildbot builds received by Milo from PubSub",
		nil,
		field.Bool("internal"),
		field.String("master"),
		field.String("builder"),
		field.Bool("finished"),
		// Status can be one of 3 options.  "New", "Replaced", "Rejected".
		field.String("status"))

	masterCounter = metric.NewCounter(
		"luci/milo/buildbot_pubsub/masters",
		"The number of buildbot master jsons received by Milo from PubSub",
		nil,
		field.Bool("internal"),
		field.String("master"),
		// Status can be one of 2 options.  "success", "failure".
		field.String("status"))
)

type buildMasterMsg struct {
	Master *buildbotMaster  `json:"master"`
	Builds []*buildbotBuild `json:"builds"`
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
	for _, builder := range master.Builders {
		// Trim out extra info in the "Changes" portion of the pending build state,
		// we don't actually need comments, files, and properties
		for _, pbs := range builder.PendingBuildStates {
			for i := range pbs.Source.Changes {
				pbs.Source.Changes[i].Comments = ""
				pbs.Source.Changes[i].Files = nil
				pbs.Source.Changes[i].Properties = nil
			}
		}
	}
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
	logging.Debugf(c, "Length of json data: %d", cw.Count)
	logging.Debugf(c, "Length of gzipped data: %d", len(entry.Data))
	return ds.Put(c, &entry)
}

// unmarshal a gzipped byte stream into a list of buildbot builds and masters.
func unmarshal(
	c context.Context, msg []byte) ([]*buildbotBuild, *buildbotMaster, error) {
	bm := buildMasterMsg{}
	if len(msg) == 0 {
		return bm.Builds, bm.Master, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(msg))
	if err != nil {
		logging.WithError(err).Errorf(c, "gzip decompression error")
		return nil, nil, err
	}
	defer reader.Close()
	d := json.NewDecoder(reader)
	if err = d.Decode(&bm); err != nil {
		logging.WithError(err).Errorf(c, "could not unmarshal message")
		return nil, nil, err
	}
	// Extract the builds out of master and append it onto builds.
	if bm.Master != nil {
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
	}
	return bm.Builds, bm.Master, nil
}

// getOSInfo fetches the os family and version of the slave the build was
// running on from the master json on a best-effort basis.
func getOSInfo(c context.Context, b *buildbotBuild, m *buildbotMaster) (
	family, version string) {
	// Fetch the master info from datastore if not provided.
	if m.Name == "" {
		logging.Infof(c, "Fetching info for master %s", b.Master)
		entry := buildbotMasterEntry{Name: b.Master}
		err := ds.Get(c, &entry)
		if err != nil {
			logging.WithError(err).Errorf(
				c, "Encountered error while fetching entry for %s", b.Master)
			return
		}
		err = decodeMasterEntry(c, &entry, m)
		if err != nil {
			logging.WithError(err).Warningf(
				c, "Failed to decode master information for OS info on master %s", b.Master)
			return
		}
		if entry.Internal && !b.Internal {
			logging.Errorf(c, "Build references an internal master, but build is not internal.")
			return
		}
	}

	s, ok := m.Slaves[b.Slave]
	if !ok {
		logging.Warningf(c, "Could not find slave %s in master %s", b.Slave, b.Master)
		return
	}
	hostInfo := map[string]string{}
	for _, v := range strings.Split(s.Host, "\n") {
		if info := strings.SplitN(v, ":", 2); len(info) == 2 {
			hostInfo[info[0]] = strings.TrimSpace(info[1])
		}
	}
	// Extract OS and OS Family
	if v, ok := hostInfo["os family"]; ok {
		family = v
	}
	if v, ok := hostInfo["os version"]; ok {
		version = v
	}
	return
}

// Marks a build as finished and expired.
func expireBuild(c context.Context, b *buildbotBuild) error {
	finished := float64(clock.Now(c).Unix())
	if b.TimeStamp != nil {
		finished = float64(*b.TimeStamp)
	}
	results := int(4) // Exception
	b.Times[1] = &finished
	b.Finished = true
	b.Results = &results
	b.Currentstep = nil
	b.Text = append(b.Text, "Build expired on Milo")
	return ds.Put(c, b)
}

func doMaster(c context.Context, master *buildbotMaster, internal bool) int {
	// Store the master json into the datastore.
	err := putDSMasterJSON(c, master, internal)
	fullname := fmt.Sprintf("master.%s", master.Name)
	if err != nil {
		logging.WithError(err).Errorf(
			c, "Could not save master in datastore %s", err)
		masterCounter.Add(c, 1, internal, fullname, "failure")
		// This is transient, we do want PubSub to retry.
		return http.StatusInternalServerError
	}
	masterCounter.Add(c, 1, internal, fullname, "success")

	// Extract current builds data out of the master json, and use it to
	// clean up expired builds.
	q := ds.NewQuery("buildbotBuild").
		Eq("finished", false).
		Eq("master", master.Name)
	builds := []*buildbotBuild{}
	err = getBuildQueryBatcher(c).GetAll(c, q, &builds)
	if err != nil {
		logging.WithError(err).Errorf(c, "Could not load current builds from master %s",
			master.Name)
		return http.StatusInternalServerError
	}
	for _, b := range builds {
		builder, ok := master.Builders[b.Buildername]
		if !ok {
			// Mark this build due to builder being removed.
			buildCounter.Add(
				c, 1, internal, b.Master, b.Buildername, b.Finished, "Expired")
			logging.Infof(c, "Expiring %s/%s/%d due to builder being removed",
				master.Name, b.Buildername, b.Number)
			err = expireBuild(c, b)
			if err != nil {
				logging.WithError(err).Errorf(c, "Could not expire build")
				return http.StatusInternalServerError
			}
			continue
		}

		found := false
		for _, bnum := range builder.CurrentBuilds {
			if b.Number == bnum {
				found = true
				break
			}
		}
		if !found {
			now := int(clock.Now(c).Unix())
			if b.TimeStamp == nil || ((*b.TimeStamp)+20*60 < now) {
				// Expire builds after 20 minutes of not getting data.
				// Mark this build due to build not current anymore.
				buildCounter.Add(
					c, 1, internal, b.Master, b.Buildername, b.Finished, "Expired")
				logging.Infof(c, "Expiring %s/%s/%d due to build not current",
					master.Name, b.Buildername, b.Number)
				err = expireBuild(c, b)
				if err != nil {
					logging.WithError(err).Errorf(c, "Could not expire build")
					return http.StatusInternalServerError
				}
			}
		}
	}
	return 0
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	statusCode := pubSubHandlerImpl(ctx.Context, ctx.Request)
	ctx.Writer.WriteHeader(statusCode)
}

// This is the actual implementation of the pubsub handler.  Returns
// a status code.  StatusOK (200) for okay (ACK implied, don't retry).
// Anything else will signal to pubsub to retry.
func pubSubHandlerImpl(c context.Context, r *http.Request) int {
	msg := common.PubSubSubscription{}
	now := int(clock.Now(c).Unix())
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		logging.WithError(err).Errorf(
			c, "Could not decode message.  %s", err)
		return http.StatusOK // This is a hard failure, we don't want PubSub to retry.
	}
	internal := true
	// Get the name of the subscription on luci-config
	settings := common.GetSettings(c)
	switch msg.Subscription {
	case settings.Buildbot.PublicSubscription:
		internal = false
	case settings.Buildbot.InternalSubscription:
		// internal = true, but that's already set.
	default:
		logging.Errorf(
			c, "Subscription name %s does not match %s or %s",
			msg.Subscription, settings.Buildbot.PublicSubscription,
			settings.Buildbot.InternalSubscription)
		// This is a configuration error. Tell PubSub to retry until we fix our
		// configs.
		return http.StatusInternalServerError
	}
	logging.Infof(
		c, "Message ID \"%s\" from subscription %s is %d bytes long",
		msg.Message.MessageID, msg.Subscription, r.ContentLength)
	bbMsg, err := msg.GetData()
	if err != nil {
		logging.WithError(err).Errorf(c, "Could not base64 decode message %s", err)
		return http.StatusOK
	}
	builds, master, err := unmarshal(c, bbMsg)
	if err != nil {
		logging.WithError(err).Errorf(c, "Could not unmarshal message %s", err)
		return http.StatusOK
	}
	logging.Infof(c, "There are %d builds", len(builds))
	if master != nil {
		logging.Infof(c, "The master name is %s", master.Name)
	} else {
		logging.Infof(c, "No master in this message")
	}
	// This is used to cache the master used for extracting OS information.
	cachedMaster := buildbotMaster{}
	// Do not use PutMulti because we might hit the 1MB limit.
	for _, build := range builds {
		if build.Master == "" {
			logging.Errorf(c, "Invalid message, missing master name")
			return http.StatusOK
		}
		existingBuild := &buildbotBuild{
			Master:      build.Master,
			Buildername: build.Buildername,
			Number:      build.Number,
		}
		buildExists := false
		if err := ds.Get(c, existingBuild); err == nil {
			if existingBuild.Finished {
				// Never replace a completed build.
				buildCounter.Add(
					c, 1, false, build.Master, build.Buildername, false, "Rejected")
				continue
			}
			buildExists = true
		}
		// Also set the finished, timestamp, and internal bit.
		build.Finished = false
		if build.TimeStamp == nil {
			build.TimeStamp = &now
		}
		if len(build.Times) == 2 && build.Times[1] != nil {
			build.Finished = true
			logging.Infof(
				c, "Recording finished build %s/%s/%d", build.Master,
				build.Buildername, build.Number)
		}
		build.Internal = internal
		// Try to get the OS information on a best-effort basis.  This assumes that all
		// builds come from one master.
		build.OSFamily, build.OSVersion = getOSInfo(c, build, &cachedMaster)
		err = ds.Put(c, build)
		if err != nil {
			if _, ok := err.(errTooBig); ok {
				// This will never work, we don't want PubSub to retry.
				logging.WithError(err).Errorf(
					c, "Could not save build to datastore, failing permanently")
				return http.StatusOK
			}
			// This is transient, we do want PubSub to retry.
			logging.WithError(err).Errorf(c, "Could not save build in datastore")
			return http.StatusInternalServerError
		}
		if buildExists {
			buildCounter.Add(
				c, 1, false, build.Master, build.Buildername, build.Finished, "Replaced")
		} else {
			buildCounter.Add(
				c, 1, false, build.Master, build.Buildername, build.Finished, "New")
		}

	}
	if master != nil {
		code := doMaster(c, master, internal)
		if code != 0 {
			return code
		}
	}
	return http.StatusOK
}
