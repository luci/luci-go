// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luci/luci-go/appengine/cmd/milo/logdog"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/client/logdog/annotee"
	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/logging"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/common/transport"
	"golang.org/x/net/context"
)

// Swarming task states..
const (
	// TaskRunning means task is running.
	TaskRunning = "RUNNING"
	// TaskPending means task didn't start yet.
	TaskPending = "PENDING"
	// TaskExpired means task expired and did not start.
	TaskExpired = "EXPIRED"
	// TaskTimedOut means task started, but took too long.
	TaskTimedOut = "TIMED_OUT"
	// TaskBotDied means task started but bot died.
	TaskBotDied = "BOT_DIED"
	// TaskCanceled means the task was canceled. See CompletedTs to determine whether it was started.
	TaskCanceled = "CANCELED"
	// TaskCompleted means task is complete.
	TaskCompleted = "COMPLETED"
)

func resolveServer(server string) string {
	// TODO(hinoka): configure this map in luci-config
	if server == "" || server == "default" || server == "dev" {
		return "chromium-swarm-dev.appspot.com"
	} else if server == "prod" {
		return "chromium-swarm.appspot.com"
	} else {
		return server
	}
}

func getSwarmingClient(c context.Context, server string) (*swarming.Service, error) {
	c, _ = context.WithTimeout(c, 60*time.Second)
	client := transport.GetClient(client.UseServiceAccountTransport(
		c, []string{"https://www.googleapis.com/auth/userinfo.email"}, nil))
	sc, err := swarming.New(client)
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", resolveServer(server))
	return sc, nil
}

func getSwarmingLog(sc *swarming.Service, taskID string) ([]byte, error) {
	// Fetch the debug file instead.
	if strings.HasPrefix(taskID, "debug:") {
		logFilename := filepath.Join("testdata", taskID[6:])
		b, err := ioutil.ReadFile(logFilename)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	tsc := sc.Task.Stdout(taskID)
	tsco, err := tsc.Do()
	if err != nil {
		return nil, err
	}
	// tsc.Do() should return an error if the http status code is not okay.
	return []byte(tsco.Output), nil
}

func getSwarmingResult(
	sc *swarming.Service, taskID string) (*swarming.SwarmingRpcsTaskResult, error) {
	if strings.HasPrefix(taskID, "debug:") {
		// Fetch the debug file instead.
		logFilename := filepath.Join("testdata", taskID[6:])
		swarmFilename := fmt.Sprintf("%s.swarm", logFilename)
		s, err := ioutil.ReadFile(swarmFilename)
		if err != nil {
			return nil, err
		}
		sr := &swarming.SwarmingRpcsTaskResult{}
		if err := json.Unmarshal(s, sr); err != nil {
			return nil, err
		}
		return sr, nil
	}
	trc := sc.Task.Result(taskID)
	srtr, err := trc.Do()
	if err != nil {
		return nil, err
	}
	return srtr, nil
}

func getSwarming(c context.Context, server string, taskID string) (
	*swarming.SwarmingRpcsTaskResult, []byte, error) {

	var log []byte
	var sr *swarming.SwarmingRpcsTaskResult
	var errLog, errRes error
	var wg sync.WaitGroup
	sc, err := func(debug bool) (*swarming.Service, error) {
		if debug {
			return nil, nil
		}
		return getSwarmingClient(c, server)
	}(strings.HasPrefix(taskID, "debug:"))
	if err != nil {
		return nil, nil, err
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		log, errLog = getSwarmingLog(sc, taskID)
	}()
	go func() {
		defer wg.Done()
		sr, errRes = getSwarmingResult(sc, taskID)
	}()
	wg.Wait()
	if errRes != nil {
		return sr, log, errRes
	}
	return sr, log, errLog
}

// TODO(hinoka): This should go in a more generic file, when milo has more
// than one page.
func getNavi(taskID string, URL string) *resp.Navigation {
	navi := &resp.Navigation{}
	navi.PageTitle = &resp.Link{
		Label: taskID,
		URL:   URL,
	}
	navi.SiteTitle = &resp.Link{
		Label: "Milo",
		URL:   "/",
	}
	return navi
}

// Given a logdog/milo step, translate it to a BuildComponent struct.
func miloBuildStep(
	c context.Context, url string, anno *miloProto.Step, name string) *resp.BuildComponent {
	comp := &resp.BuildComponent{}
	asc := anno.GetStepComponent()
	comp.Label = asc.Name
	switch asc.Status {
	case miloProto.Status_RUNNING:
		comp.Status = resp.Running

	case miloProto.Status_SUCCESS:
		comp.Status = resp.Success

	case miloProto.Status_FAILURE:
		if anno.GetFailureDetails() != nil {
			switch anno.GetFailureDetails().Type {
			case miloProto.FailureDetails_INFRA:
				comp.Status = resp.InfraFailure

			case miloProto.FailureDetails_DM_DEPENDENCY_FAILED:
				comp.Status = resp.DependencyFailure

			default:
				comp.Status = resp.Failure
			}
		} else {
			comp.Status = resp.Failure
		}

	case miloProto.Status_EXCEPTION:
		comp.Status = resp.InfraFailure

		// Missing the case of waiting on unfinished dependency...
	default:
		comp.Status = resp.NotRun
	}
	// Sub link is for one link per log that isn't stdio.
	for _, link := range asc.GetOtherLinks() {
		lds := link.GetLogdogStream()
		if lds == nil {
			logging.Warningf(c, "Warning: %v of %v has an empty logdog stream.", link, asc)
			continue // DNE???
		}
		shortName := lds.Name[5 : len(lds.Name)-2]
		if strings.HasSuffix(lds.Name, "annotations") || strings.HasSuffix(lds.Name, "stdio") {
			// Skip the special ones.
			continue
		}
		newLink := &resp.Link{
			Label: shortName,
			URL:   strings.Join([]string{url, lds.Name}, "/"),
		}
		comp.SubLink = append(comp.SubLink, newLink)
	}

	// Main link is a link to the stdio.
	comp.MainLink = &resp.Link{
		Label: "stdio",
		URL:   strings.Join([]string{url, name, "stdio"}, "/"),
	}

	// This should always be a step.
	comp.Type = resp.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timeswamapts
	comp.Started = asc.Started.Time().Format(time.RFC3339)

	// This should be the exact same thing.
	comp.Text = asc.Text

	return comp
}

func swarmingProperties(sr *swarming.SwarmingRpcsTaskResult) *resp.PropertyGroup {
	props := &resp.PropertyGroup{GroupName: "Swarming"}
	if len(sr.CostsUsd) == 1 {
		props.Property = append(props.Property, &resp.Property{
			Key:   "Cost of job (USD)",
			Value: fmt.Sprintf("$%.2f", sr.CostsUsd[0]),
		})
	}
	if sr.State == TaskCompleted || sr.State == TaskTimedOut {
		props.Property = append(props.Property, &resp.Property{
			Key:   "Exit Code",
			Value: fmt.Sprintf("%d", sr.ExitCode),
		})
	}
	return props
}

func swarmingTags(sr *swarming.SwarmingRpcsTaskResult) *resp.PropertyGroup {
	props := &resp.PropertyGroup{GroupName: "Swarming Tags"}
	for _, s := range sr.Tags {
		sp := strings.SplitN(s, ":", 2)
		var k, v string
		k = sp[0]
		if len(sp) == 2 {
			v = sp[1]
		}
		props.Property = append(props.Property, &resp.Property{
			Key:   k,
			Value: v,
		})
	}
	return props
}

func addSwarmingToBuild(
	c context.Context, sr *swarming.SwarmingRpcsTaskResult, build *resp.MiloBuild) {
	// Specify the result.
	if sr.State == "RUNNING" {
		build.Summary.Status = resp.Running
	} else if sr.State == "PENDING" {
		build.Summary.Status = resp.NotRun
	} else if sr.InternalFailure == true || sr.State == "BOT_DIED" || sr.State == "EXPIRED" || sr.State == "TIMED_OUT" {
		build.Summary.Status = resp.InfraFailure
	} else if sr.Failure == true || sr.State == "CANCELLED" {
		// Cancelled build is user action, so it is not an infra failure.
		build.Summary.Status = resp.Failure
	} else {
		build.Summary.Status = resp.Success
	}

	// Extract more swarming specific information into the properties.
	build.PropertyGroup = append(build.PropertyGroup, swarmingProperties(sr))
	build.PropertyGroup = append(build.PropertyGroup, swarmingTags(sr))

	// Build times.  Swarming timestamps are RFC3339Nano without the timezone
	// information, which is assumed to be UTC, so we fix it here.
	build.Summary.Started = fmt.Sprintf("%sZ", sr.StartedTs)
	if sr.CompletedTs != "" {
		build.Summary.Finished = fmt.Sprintf("%sZ", sr.CompletedTs)
	}
	build.Summary.Duration = uint64(sr.Duration)
}

// Takes in an annotated log and returns a fully populated set of logdog streams
func streamsFromAnnotatedLog(ctx context.Context, log []byte) (*logdog.Streams, error) {
	c := &memoryClient{}
	p := annotee.New(ctx, annotee.Options{
		Client:                 c,
		MetadataUpdateInterval: -1, // Neverrrrrr send incr updates.
		Offline:                true,
	})
	defer p.Finish()

	is := annotee.Stream{
		Reader:           bytes.NewBuffer(log),
		Name:             types.StreamName("stdout"),
		Annotate:         true,
		StripAnnotations: true,
	}
	// If this ever has more than one stream then memoryClient needs to become
	// goroutine safe
	if err := p.RunStreams([]*annotee.Stream{&is}); err != nil {
		return nil, err
	}
	p.Finish()
	return c.ToLogDogStreams()
}

func swarmingBuildImpl(c context.Context, URL string, server string, taskID string) (*resp.MiloBuild, error) {
	// Fetch the data from Swarming
	sr, body, err := getSwarming(c, server, taskID)
	if err != nil {
		return nil, err
	}

	allowMilo := false
	for _, t := range sr.Tags {
		if t == "allow_milo:1" {
			allowMilo = true
			break
		}
	}
	if !allowMilo {
		return nil, fmt.Errorf("Not A Milo Job")
	}

	build := &resp.MiloBuild{}
	addSwarmingToBuild(c, sr, build)

	// Decode the data using annotee. The logdog stream returned here is assumed
	// to be consistent, which is why the following block of code are not
	// expected to ever err out.
	lds, err := streamsFromAnnotatedLog(c, body)
	if err != nil {
		build.Components = []*resp.BuildComponent{{
			Type:   resp.Summary,
			Label:  "milo annotation parser",
			Text:   []string{err.Error()},
			Status: resp.InfraFailure,
			SubLink: []*resp.Link{{
				Label: "swarming task",
				URL:   taskPageURL(resolveServer(server), taskID),
			}},
		}}
	} else {
		logdog.AddLogDogToBuild(c, URL, lds, build)
	}

	return build, nil
}

// taskPageURL returns a URL to a human-consumable page of a swarming task.
func taskPageURL(swarmingHostname, taskID string) string {
	return fmt.Sprintf("https://%s/user/task/%s", swarmingHostname, taskID)
}
