// Copyright 2016 The LUCI Authors.
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

package buildbot

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"
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

	allMasterTimer = metric.NewFloat(
		"luci/milo/buildbot_pubsub/last_updated",
		"Seconds since the master was last updated.",
		nil,
		field.String("master"))
)

type buildMasterMsg struct {
	Master *buildbot.Master  `json:"master"`
	Builds []*buildbot.Build `json:"builds"`
}

// unmarshal a gzipped byte stream into a list of buildbot builds and masters.
func unmarshal(
	c context.Context, msg []byte) ([]*buildbot.Build, *buildbot.Master, error) {
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
func getOSInfo(c context.Context, b *buildbot.Build, m *buildstore.Master) (
	family, version string) {
	// Fetch the master info from datastore if not provided.
	if m.Name == "" {
		logging.Infof(c, "Fetching info for master %s", b.Master)
		freshMaster, err := buildstore.GetMaster(c, b.Master, false)
		if err != nil {
			logging.WithError(err).Errorf(c, "fetching master %q", b.Master)
			return
		}
		if freshMaster.Internal && !b.Internal {
			logging.Errorf(c, "Build references an internal master, but build is not internal.")
			return
		}
		*m = *freshMaster
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

func saveMaster(c context.Context, master *buildbot.Master, internal bool) int {
	// Store the master in the storage.
	expireCallback := func(b *buildbot.Build, reason string) {
		logging.Infof(c, "Expiring %s/%s/%d due to %s",
			master.Name, b.Buildername, b.Number, reason)
		buildCounter.Add(
			c, 1, internal, b.Master, b.Buildername, b.Finished, "Expired")
	}
	err := buildstore.SaveMaster(c, master, internal, expireCallback)
	fullname := fmt.Sprintf("master.%s", master.Name)
	if err != nil {
		logging.WithError(err).Errorf(c, "Could not import master")
		masterCounter.Add(c, 1, internal, fullname, "failure")
		if buildstore.TooBigTag.In(err) {
			// FIXME: there should have been a metric and alert.
			return 0
		}
		if buildstore.ImportRejectedTag.In(err) {
			// Permanent failure, don't retry.
			return 0
		}
		// This is transient, we do want PubSub to retry.
		return http.StatusInternalServerError
	}
	masterCounter.Add(c, 1, internal, fullname, "success")
	return 0
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	statusCode := pubSubHandlerImpl(ctx.Context, ctx.Request)
	ctx.Writer.WriteHeader(statusCode)
}

// StatsHandler is a cron endpoint that sends stats periodically.
func StatsHandler(c context.Context) error {
	masters, err := buildstore.AllMasters(c, false)
	if err != nil {
		return errors.Annotate(err, "failed to fetch masters").Err()
	}
	now := clock.Now(c)
	for _, m := range masters {
		allMasterTimer.Set(c, now.Sub(m.Modified).Seconds(), m.Name)
	}
	return nil
}

// This is the actual implementation of the pubsub handler.  Returns
// a status code.  StatusOK (200) for okay (ACK implied, don't retry).
// Anything else will signal to pubsub to retry.
func pubSubHandlerImpl(c context.Context, r *http.Request) int {
	msg := common.PubSubSubscription{}
	now := clock.Now(c)
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
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
	var cachedMaster buildstore.Master
	bail := false
	for _, build := range builds {
		if build.Master == "" {
			logging.Errorf(c, "Invalid message, missing master name")
			return http.StatusOK
		}

		// Also set the finished, timestamp, and internal bit.
		build.Finished = false
		if build.TimeStamp.IsZero() {
			build.TimeStamp.Time = now
		}
		if !build.Times.Finish.IsZero() {
			build.Finished = true
		}
		build.Internal = internal
		// Try to get the OS information on a best-effort basis.  This assumes that all
		// builds come from one master.
		build.OSFamily, build.OSVersion = getOSInfo(c, build, &cachedMaster)
		replaced, err := buildstore.SaveBuild(c, build)
		switch {
		case buildstore.ImportRejectedTag.In(err):
			// Most probably, build has already completed.
			buildCounter.Add(
				c, 1, false, build.Master, build.Buildername, build.Finished, "Rejected")
			logging.Warningf(c, "import of %s/%s/%d rejected", build.Master, build.Buildername, build.Number)
			continue
		case buildstore.TooBigTag.In(err):
			// This will never work, we don't want PubSub to retry.
			logging.WithError(err).Errorf(
				c, "Could not save build to datastore, failing permanently")
			bail = true // This large build could potentially cause issues, need to bail after this.
			continue
		case err != nil:
			// This is transient, we do want PubSub to retry.
			logging.WithError(err).Errorf(c, "Could not save build in datastore")
			return http.StatusInternalServerError
		}

		// TODO(iannucci): make these actions match the metric for buildbucket
		//   builds.
		status := "New"
		if replaced {
			status = "Replaced"
		}
		buildCounter.Add(
			c, 1, false, build.Master, build.Buildername, build.Finished, status)
	}
	if bail {
		logging.Infof(c, "Skipping saving master data due to potential OOM")
		return http.StatusOK
	}
	if master != nil {
		code := saveMaster(c, master, internal)
		if code != 0 {
			return code
		}
	}
	return http.StatusOK
}
