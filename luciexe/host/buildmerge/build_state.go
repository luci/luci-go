// Copyright 2019 The LUCI Authors.
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

package buildmerge

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

type buildState struct {
	mu     sync.Mutex
	latest *bbpb.Build

	// closed is set to true when the build state is terminated and will recieve
	// no more updates.
	closed bool

	// invalid is set to true when the interior structure (i.e. Steps) of latest
	// contains invalid data and shouldn't be inspected.
	invalid bool

	logdogNS string

	merger *agent
}

func newBuildState(merger *agent, namespace string, err error) *buildState {
	ret := &buildState{
		merger:   merger,
		logdogNS: namespace,
	}
	if err != nil {
		ret.latest = &bbpb.Build{}
		setErrorOnBuild(ret.latest, err)
		ret.closed = true
	}
	return ret
}

func (bs *buildState) GetLatest() (latest *bbpb.Build, valid bool) {
	if bs != nil {
		bs.mu.Lock()
		latest = bs.latest
		valid = !bs.invalid
		bs.mu.Unlock()
	}
	if latest == nil {
		latest = &bbpb.Build{
			SummaryMarkdown: "build.proto not found",
			Status:          bbpb.Status_SCHEDULED,
		}
	}
	return
}

func (bs *buildState) StepCount() (ret int) {
	if bs != nil {
		bs.mu.Lock()
		defer bs.mu.Unlock()
		ret = len(bs.latest.GetSteps())
	}
	return
}

// This callback function blocks the stream until it returns; buildState
// cannot change until we change it.
func (bs *buildState) parse(entry *logpb.LogEntry) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.closed {
		return
	}

	now := bs.merger.clkNow()
	if entry == nil {
		bs.closed = true
		if bs.latest == nil {
			bs.latest = &bbpb.Build{
				SummaryMarkdown: "Never recieved any build data.",
				Status:          bbpb.Status_INFRA_FAILURE,
			}
		} else {
			processFinalBuild(now, bs.latest)
		}
		bs.merger.informNewData()
		return
	}

	if !bs.merger.collectingData() {
		return
	}

	// doParse
	dg := entry.Content.(*logpb.LogEntry_Datagram).Datagram
	// We call AddStreamRegistrationCallback with wrap==true, so this is never
	// partial.

	var parsedBuild *bbpb.Build
	err := func() error {
		build := &bbpb.Build{}
		if err := proto.Unmarshal(dg.Data, build); err != nil {
			return errors.Annotate(err, "parsing Build").Err()
		}
		parsedBuild = build

		for _, step := range parsedBuild.Steps {
			for _, log := range step.Logs {
				if err := types.StreamName(log.Url).Validate(); err != nil {
					step.Status = bbpb.Status_INFRA_FAILURE
					step.SummaryMarkdown += fmt.Sprintf("bad log url: %q", log.Url)
					return errors.Annotate(
						err, "step[%q].logs[%q].Url = %q", step.Name, log.Name, log.Url).Err()
				}

				log.Url, log.ViewUrl = bs.merger.calculateURLs(bs.logdogNS, log.Url)
			}
		}
		return nil
	}()
	if err != nil {
		if parsedBuild == nil {
			if bs.latest == nil {
				parsedBuild = &bbpb.Build{}
			} else {
				// make a shallow copy of the latest build
				buildVal := *bs.latest
				parsedBuild = &buildVal
			}
		}
		setErrorOnBuild(parsedBuild, err)
		processFinalBuild(now, parsedBuild)
		bs.closed = true
		bs.invalid = true
	}
	parsedBuild.UpdateTime = now

	bs.latest = parsedBuild
	bs.merger.informNewData()
}
