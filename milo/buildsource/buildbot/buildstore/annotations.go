// Copyright 2017 The LUCI Authors.
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

package buildstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
)

// This file implements conversion of annotations to buildbot steps and
// properties.

var reLineBreak = regexp.MustCompile(`<br */?>`)

var errAnnotationNotFound = errors.New("annotation not found")

// fetchAnnotationProto fetches an annotation proto from LogDog.
// If the stream is not found, returns errAnnotationNotFound.
func fetchAnnotationProto(c context.Context, addr *types.StreamAddr) (*miloProto.Step, error) {
	// The implementation avoids using existing Milo code because the latter is
	// likely to change and because it does things that we don't need here e.g.
	// caching that wouldn't apply anyway, etc.
	// Instead we use LogDog client directly, and thus make it simpler to
	// reason what this function actually does.
	// This is not performance critical code.

	// Create a LogDog client.
	transport, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.New("failed to get transport for LogDog server")
	}
	client := coordinator.NewClient(&prpc.Client{
		C:    &http.Client{Transport: transport},
		Host: addr.Host,
	})

	// Load the last datagram.
	var state coordinator.LogStream
	logEntry, err := client.Stream(addr.Project, addr.Path).
		Tail(c, coordinator.WithState(&state), coordinator.Complete())
	switch {
	case err == coordinator.ErrNoSuchStream:
		return nil, errAnnotationNotFound
	case err == coordinator.ErrNoAccess:
		// Tag with Milo internal tags.
		return nil, errors.Annotate(err, "getting logdog stream").Tag(common.CodeNoAccess).Err()
	case err != nil:
		return nil, err
	case state.Desc.StreamType != logpb.StreamType_DATAGRAM:
		return nil, errors.New("not a datagram stream")
	case logEntry == nil:
		return nil, errAnnotationNotFound
	}

	// Decode the datagram as an annotation proto.
	var res miloProto.Step
	if err := proto.Unmarshal(logEntry.GetDatagram().Data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// annotationConverter converts annotation steps to buildbot steps.
type annotationConverter struct {
	logdogServer, logdogPrefix string
	buildCompletedTime         time.Time
}

// addSteps converts annotation substeps to buildbot steps and appends them
// dest.
// c is used only for logging.
func (ac *annotationConverter) addSteps(c context.Context, dest *[]buildbot.Step, src []*miloProto.Step_Substep, stepNamePrefix string) error {
	for _, substep := range src {
		stepSrc := substep.GetStep()
		if stepSrc == nil {
			return errors.Reason("unexpected substep type %T", substep.Substep).Err()
		}

		stepDst, err := ac.step(c, stepSrc)
		if err != nil {
			return errors.Annotate(err, "could not convert step %q", stepNamePrefix+stepDst.Name).Err()
		}
		stepDst.Name = stepNamePrefix + stepDst.Name
		*dest = append(*dest, *stepDst)
		ac.addSteps(c, dest, stepSrc.Substep, stepDst.Name+".")
	}
	return nil
}

// step converts an annotation step to a buildbot step.
func (ac *annotationConverter) step(c context.Context, src *miloProto.Step) (*buildbot.Step, error) {
	// This implementation is based on
	// https://chromium.googlesource.com/infra/luci/luci-go/+/7ad046489c578e339b873886d6973abbe43cc137/milo/buildsource/rawpresentation/logDogBuild.go#47

	res := &buildbot.Step{
		Name:       src.Name,
		IsStarted:  true,
		IsFinished: src.Ended != nil,
	}

	// Convert step result.
	switch {
	case src.Status == miloProto.Status_SUCCESS:
		res.Results.Result = buildbot.Success

	case src.Status == miloProto.Status_FAILURE:
		res.Results.Result = buildbot.Failure
		if fd := src.GetFailureDetails(); fd != nil {
			if fd.Type != miloProto.FailureDetails_GENERAL {
				res.Results.Result = buildbot.Exception
			}
			if fd.Text != "" {
				res.Text = append(res.Text, fd.Text)
			}
		}

	case !ac.buildCompletedTime.IsZero():
		res.Results.Result = buildbot.Exception

	default:
		res.Results.Result = buildbot.NoResult
	}

	// annotee never initializes src.Link
	var allLinks []*miloProto.Link
	if src.StdoutStream != nil {
		allLinks = append(allLinks, &miloProto.Link{
			Label: "stdout",
			Value: &miloProto.Link_LogdogStream{src.StdoutStream},
		})
	}
	if src.StderrStream != nil {
		allLinks = append(allLinks, &miloProto.Link{
			Label: "stderr",
			Value: &miloProto.Link_LogdogStream{src.StdoutStream},
		})
	}
	allLinks = append(allLinks, src.OtherLinks...)
	for _, l := range allLinks {
		log, err := ac.log(l)
		if err != nil {
			logging.WithError(err).Errorf(c, "could not convert link %q to buildbot", l.Label)
			continue
		}
		res.Logs = append(res.Logs, *log)
	}

	// Timestamps
	if src.Started != nil {
		var err error
		res.Times.Start.Time, err = ptypes.Timestamp(src.Started)
		if err != nil {
			return nil, errors.Annotate(err, "invalid start time").Err()
		}
		if src.Ended != nil {
			res.Times.Finish.Time, err = ptypes.Timestamp(src.Ended)
			if err != nil {
				return nil, errors.Annotate(err, "invalid end time").Err()
			}
		} else {
			res.Times.Finish.Time = ac.buildCompletedTime
		}
	}

	for _, line := range src.Text {
		// src.Text may contain <br> ;(
		res.Text = append(res.Text, reLineBreak.Split(line, -1)...)
	}
	return res, nil
}

func (ac *annotationConverter) log(src *miloProto.Link) (*buildbot.Log, error) {
	// This implementation is based on
	// https://chromium.googlesource.com/infra/luci/luci-go/+/7ad046489c578e339b873886d6973abbe43cc137/milo/buildsource/swarming/build.go#798

	// Milo ignores miloProto.Link.AliasLink
	// Also we don't have it in practice.

	log := &buildbot.Log{Name: src.Label}
	switch v := src.Value.(type) {
	case *miloProto.Link_LogdogStream:
		// This is the typical case.

		if v.LogdogStream == nil {
			return nil, errors.Reason("LogdogStream link is empty").Err()
		}
		stream := *v.LogdogStream
		if stream.Server == "" {
			stream.Server = ac.logdogServer
		}
		if stream.Prefix == "" {
			stream.Prefix = ac.logdogPrefix
		}
		log.URL = fmt.Sprintf("https://%s/v/?s=%s", stream.Server, url.QueryEscape(stream.Prefix+"/+/"+stream.Name))

	case *miloProto.Link_Url:
		log.URL = v.Url

	default:
		return nil, errors.Reason("unexpected link value type %T: %v", v, v).Err()
	}
	return log, nil
}

func extractProperties(s *miloProto.Step) []*buildbot.Property {
	props := map[string]*buildbot.Property{}

	var extract func(s *miloProto.Step)
	extract = func(s *miloProto.Step) {
		for _, p := range s.Property {
			var v interface{}
			if err := json.Unmarshal([]byte(p.Value), &v); err != nil {
				// p.Value was never properly documented as JSON, so
				// there is a possibility that someone puts strings
				v = p.Value
			}

			props[p.Name] = &buildbot.Property{
				Name:   p.Name,
				Value:  v,
				Source: s.Name,
			}
		}
		for _, substep := range s.GetSubstep() {
			if ss := substep.GetStep(); ss != nil {
				extract(ss)
			}
		}
	}

	extract(s)

	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)

	ret := make([]*buildbot.Property, len(names))
	for i, n := range names {
		ret[i] = props[n]
	}
	return ret
}
