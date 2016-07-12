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

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/logdog"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/client/logdog/annotee"
	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/transport"
)

// SwarmingTimeLayout is time layout used by swarming.
const SwarmingTimeLayout = "2006-01-02T15:04:05.999999999"

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

func getSwarmingClient(c context.Context, server string) (*swarming.Service, error) {
	c, _ = context.WithTimeout(c, 60*time.Second)
	client := transport.GetClient(client.UseServiceAccountTransport(c, nil, nil))
	sc, err := swarming.New(client)
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", server)
	return sc, nil
}

func getDebugTaskOutput(taskID string) (string, error) {
	// Read the debug file instead.
	logFilename := filepath.Join("testdata", taskID)
	b, err := ioutil.ReadFile(logFilename)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func getTaskOutput(sc *swarming.Service, taskID string) (string, error) {
	res, err := sc.Task.Stdout(taskID).Do()
	if err != nil {
		return "", err
	}
	return res.Output, nil
}

func getDebugSwarmingResult(
	taskID string) (*swarming.SwarmingRpcsTaskResult, error) {

	logFilename := filepath.Join("testdata", taskID)
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

func getSwarming(c context.Context, server string, taskID string) (
	*swarming.SwarmingRpcsTaskResult, string, error) {

	var log string
	var sr *swarming.SwarmingRpcsTaskResult
	var errLog, errRes error

	// Detour: Return debugging results, useful for development.
	if server == "debug" {
		sr, errRes = getDebugSwarmingResult(taskID)
		log, errLog = getDebugTaskOutput(taskID)
	} else {
		sc, err := getSwarmingClient(c, server)
		if err != nil {
			return nil, "", err
		}

		var wg sync.WaitGroup
		wg.Add(2) // Getting log and result can happen concurrently.  Wait for both.

		go func() {
			defer wg.Done()
			log, errLog = getTaskOutput(sc, taskID)
		}()
		go func() {
			defer wg.Done()
			sr, errRes = sc.Task.Result(taskID).Do()
		}()
		wg.Wait()
	}

	if errRes != nil {
		// Swarming result errors take priority.
		return sr, log, errRes
	}

	switch sr.State {
	case TaskCompleted, TaskRunning, TaskCanceled:
	default:
		//  Ignore log errors if the task might be pending, timed out, expired, etc.
		if errLog != nil {
			errLog = nil
			log = ""
		}
	}
	return sr, log, errLog
}

func taskProperties(sr *swarming.SwarmingRpcsTaskResult) *resp.PropertyGroup {
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

func tagsToProperties(tags []string) *resp.PropertyGroup {
	props := &resp.PropertyGroup{GroupName: "Swarming Tags"}
	for _, t := range tags {
		if t == "" {
			continue
		}
		parts := strings.SplitN(t, ":", 2)
		p := &resp.Property{
			Key: parts[0],
		}
		if len(parts) == 2 {
			p.Value = parts[1]
		}
		props.Property = append(props.Property, p)
	}
	return props
}

func taskToBuild(c context.Context, server string, sr *swarming.SwarmingRpcsTaskResult) (*resp.MiloBuild, error) {
	build := &resp.MiloBuild{
		Summary: resp.BuildComponent{
			Source: &resp.Link{
				Label: "Task " + sr.TaskId,
				URL:   taskPageURL(server, sr.TaskId),
			},
		},
	}

	switch sr.State {
	case TaskRunning:
		build.Summary.Status = resp.Running

	case TaskPending:
		build.Summary.Status = resp.NotRun

	case TaskExpired, TaskTimedOut, TaskBotDied:
		build.Summary.Status = resp.InfraFailure
		switch sr.State {
		case TaskExpired:
			build.Summary.Text = append(build.Summary.Text, "Task expired")
		case TaskTimedOut:
			build.Summary.Text = append(build.Summary.Text, "Task timed out")
		case TaskBotDied:
			build.Summary.Text = append(build.Summary.Text, "Bot died")
		}

	case TaskCanceled:
		// Cancelled build is user action, so it is not an infra failure.
		build.Summary.Status = resp.Failure
		build.Summary.Text = append(build.Summary.Text, "Task cancelled by user")

	case TaskCompleted:

		switch {
		case sr.InternalFailure:
			build.Summary.Status = resp.InfraFailure
		case sr.Failure:
			build.Summary.Status = resp.Failure
		default:
			build.Summary.Status = resp.Success
		}

	default:
		return nil, fmt.Errorf("unknown swarming task state %q", sr.State)
	}

	// Extract more swarming specific information into the properties.
	if props := taskProperties(sr); len(props.Property) > 0 {
		build.PropertyGroup = append(build.PropertyGroup, props)
	}
	if props := tagsToProperties(sr.Tags); len(props.Property) > 0 {
		build.PropertyGroup = append(build.PropertyGroup, props)
	}

	if sr.BotId != "" {
		build.Summary.Bot = &resp.Link{
			Label: sr.BotId,
			URL:   botPageURL(server, sr.BotId),
		}
	}

	var err error
	if sr.StartedTs != "" {
		build.Summary.Started, err = time.Parse(SwarmingTimeLayout, sr.StartedTs)
		if err != nil {
			return nil, fmt.Errorf("invalid task StartedTs: %s", err)
		}
	}
	if sr.CompletedTs != "" {
		build.Summary.Finished, err = time.Parse(SwarmingTimeLayout, sr.StartedTs)
		if err != nil {
			return nil, fmt.Errorf("invalid task CompletedTs: %s", err)
		}
	}
	if sr.Duration != 0 {
		build.Summary.Duration = time.Duration(sr.Duration * float64(time.Second))
	} else if sr.State == TaskRunning {
		now := clock.Now(c)
		if build.Summary.Started.Before(now) {
			build.Summary.Duration = now.Sub(build.Summary.Started)
		}
	}

	return build, nil
}

// streamsFromAnnotatedLog takes in an annotated log and returns a fully
// populated set of logdog streams
func streamsFromAnnotatedLog(ctx context.Context, log string) (*logdog.Streams, error) {
	c := &memoryClient{}
	p := annotee.New(ctx, annotee.Options{
		Client:                 c,
		MetadataUpdateInterval: -1, // Neverrrrrr send incr updates.
		Offline:                true,
	})

	is := annotee.Stream{
		Reader:           bytes.NewBufferString(log),
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

func swarmingBuildImpl(c context.Context, linkBase, server, taskID string) (*resp.MiloBuild, error) {
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

	build, err := taskToBuild(c, server, sr)
	if err != nil {
		return nil, err
	}

	// Decode the data using annotee. The logdog stream returned here is assumed
	// to be consistent, which is why the following block of code are not
	// expected to ever err out.
	if body != "" {
		lds, err := streamsFromAnnotatedLog(c, body)
		if err != nil {
			build.Components = []*resp.BuildComponent{{
				Type:   resp.Summary,
				Label:  "Milo annotation parser",
				Text:   []string{err.Error()},
				Status: resp.InfraFailure,
				SubLink: []*resp.Link{{
					Label: "swarming task",
					URL:   taskPageURL(server, taskID),
				}},
			}}
		} else {
			logdog.AddLogDogToBuild(c, linkBase, lds, build)
		}
	}

	return build, nil
}

// taskPageURL returns a URL to a human-consumable page of a swarming task.
// Supports server aliases.
func taskPageURL(swarmingHostname, taskID string) string {
	return fmt.Sprintf("https://%s/user/task/%s", swarmingHostname, taskID)
}

// botPageURL returns a URL to a human-consumable page of a swarming bot.
// Supports server aliases.
func botPageURL(swarmingHostname, botID string) string {
	return fmt.Sprintf("https://%s/restricted/bot/%s", swarmingHostname, botID)
}
