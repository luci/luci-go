// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/common/sync/parallel"
	"github.com/luci/luci-go/logdog/client/annotee"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/logdog"
	"github.com/luci/luci-go/server/auth"
)

// errNotMiloJob is returned if a Swarming task is fetched that does not self-
// identify as a Milo job.
var errNotMiloJob = errors.New("Not a Milo Job")

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
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	sc, err := swarming.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", server)
	return sc, nil
}

// swarmingService is an interface that fetches data from Swarming.
//
// In production, this is fetched from a Swarming server. For testing, this can
// be replaced with a mock.
type swarmingService interface {
	getHost() string
	getSwarmingResult(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error)
	getSwarmingRequest(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskRequest, error)
	getTaskOutput(c context.Context, taskID string) (string, error)
}

type prodSwarmingService struct {
	host   string
	client *swarming.Service
}

func newProdService(c context.Context, server string) (*prodSwarmingService, error) {
	client, err := getSwarmingClient(c, server)
	if err != nil {
		return nil, err
	}
	return &prodSwarmingService{
		host:   server,
		client: client,
	}, nil
}

func (svc *prodSwarmingService) getHost() string { return svc.host }

func (svc *prodSwarmingService) getSwarmingResult(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error) {
	return svc.client.Task.Result(taskID).Context(c).Do()
}

func (svc *prodSwarmingService) getTaskOutput(c context.Context, taskID string) (string, error) {
	stdout, err := svc.client.Task.Stdout(taskID).Context(c).Do()
	if err != nil {
		return "", err
	}
	return stdout.Output, nil
}

func (svc *prodSwarmingService) getSwarmingRequest(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskRequest, error) {
	return svc.client.Task.Request(taskID).Context(c).Do()
}

type swarmingFetchParams struct {
	fetchReq bool
	fetchRes bool
	fetchLog bool
}

type swarmingFetchResult struct {
	req *swarming.SwarmingRpcsTaskRequest
	res *swarming.SwarmingRpcsTaskResult
	log string
}

// swarmingFetch fetches (in parallel) the components that it is configured to
// fetch.
//
// After fetching, an ACL check is performed to confirm that the user is
// permitted to view the resulting data. If this check fails, get returns
// errNotMiloJob.
//
// TODO(hinoka): Make this ACL check something more specific than the
// resence of the "allow_milo" dimension.
func swarmingFetch(c context.Context, svc swarmingService, taskID string, req swarmingFetchParams) (
	*swarmingFetchResult, error) {

	// logErr is managed separately from other fetch errors, since in some
	// situations it's acceptable to not have a log stream.
	var logErr error
	var fr swarmingFetchResult

	err := parallel.FanOutIn(func(workC chan<- func() error) {
		if req.fetchReq {
			workC <- func() (err error) {
				fr.req, err = svc.getSwarmingRequest(c, taskID)
				return
			}
		}

		if req.fetchRes {
			workC <- func() (err error) {
				fr.res, err = svc.getSwarmingResult(c, taskID)
				return
			}
		}

		if req.fetchLog {
			workC <- func() error {
				fr.log, logErr = svc.getTaskOutput(c, taskID)
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	// Current ACL implementation: error if this is not a Milo job.
	switch {
	case req.fetchReq:
		if !isMiloJob(fr.req.Tags) {
			return nil, errNotMiloJob
		}
	case req.fetchRes:
		if !isMiloJob(fr.res.Tags) {
			return nil, errNotMiloJob
		}
	default:
		// No metadata to decide if this is a Milo job, so assume that it is not.
		return nil, errNotMiloJob
	}

	if req.fetchRes && logErr != nil {
		switch fr.res.State {
		case TaskCompleted, TaskRunning, TaskCanceled:
		default:
			//  Ignore log errors if the task might be pending, timed out, expired, etc.
			if err != nil {
				fr.log = ""
				logErr = nil
			}
		}
	}
	return &fr, logErr
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

// tagValue returns a value of the first tag matching the tag key. If not found
// returns "".
func tagValue(tags []string, key string) string {
	prefix := key + ":"
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return strings.TrimPrefix(t, prefix)
		}
	}
	return ""
}

// addBuilderLink adds a link to the buildbucket builder view.
func addBuilderLink(c context.Context, build *resp.MiloBuild, swarmingHostname string, sr *swarming.SwarmingRpcsTaskResult) {
	bbHost := tagValue(sr.Tags, "buildbucket_hostname")
	bucket := tagValue(sr.Tags, "buildbucket_bucket")
	builder := tagValue(sr.Tags, "builder")
	if bucket == "" {
		logging.Errorf(
			c, "Could not extract buidlbucket bucket from task %s",
			taskPageURL(swarmingHostname, sr.TaskId))
	}
	if builder == "" {
		logging.Errorf(
			c, "Could not extract builder name from task %s",
			taskPageURL(swarmingHostname, sr.TaskId))
	}
	if bucket != "" && builder != "" {
		build.Summary.ParentLabel = &resp.Link{
			Label: builder,
			URL:   fmt.Sprintf("/buildbucket/%s/%s?server=%s", bucket, builder, bbHost),
		}
	}
}

// addBanner adds an OS banner derived from "os" swarming tag, if present.
func addBanner(build *resp.MiloBuild, sr *swarming.SwarmingRpcsTaskResult) {
	var os, ver string
	for _, t := range sr.Tags {
		value := strings.TrimPrefix(t, "os:")
		if value == t {
			// t does not have the prefix
			continue
		}
		parts := strings.SplitN(value, "-", 2)
		if len(parts) == 2 {
			os = parts[0]
			ver = parts[1]
			break
		}
	}

	var base resp.LogoBase
	switch os {
	case "Ubuntu":
		base = resp.Ubuntu
	case "Windows":
		base = resp.Windows
	case "Mac":
		base = resp.OSX
	case "Android":
		base = resp.Android
	default:
		return
	}

	build.Summary.Banner = &resp.LogoBanner{
		OS: []resp.Logo{{
			LogoBase: base,
			Subtitle: ver,
			Count:    1,
		}},
	}
}

// addTaskToMiloStep augments a Milo Annotation Protobuf with state from the
// Swarming task.
func addTaskToMiloStep(c context.Context, server string, sr *swarming.SwarmingRpcsTaskResult, step *miloProto.Step) error {
	step.Link = &miloProto.Link{
		Label: "Task " + sr.TaskId,
		Value: &miloProto.Link_Url{
			Url: taskPageURL(server, sr.TaskId),
		},
	}

	switch sr.State {
	case TaskRunning:
		step.Status = miloProto.Status_RUNNING

	case TaskPending:
		step.Status = miloProto.Status_PENDING

	case TaskExpired, TaskTimedOut, TaskBotDied:
		step.Status = miloProto.Status_FAILURE

		switch sr.State {
		case TaskExpired:
			step.FailureDetails = &miloProto.FailureDetails{
				Type: miloProto.FailureDetails_EXPIRED,
				Text: "Task expired",
			}
		case TaskTimedOut:
			step.FailureDetails = &miloProto.FailureDetails{
				Type: miloProto.FailureDetails_INFRA,
				Text: "Task timed out",
			}
		case TaskBotDied:
			step.FailureDetails = &miloProto.FailureDetails{
				Type: miloProto.FailureDetails_INFRA,
				Text: "Bot died",
			}
		}

	case TaskCanceled:
		// Cancelled build is user action, so it is not an infra failure.
		step.Status = miloProto.Status_FAILURE
		step.FailureDetails = &miloProto.FailureDetails{
			Type: miloProto.FailureDetails_CANCELLED,
			Text: "Task cancelled by user",
		}

	case TaskCompleted:

		switch {
		case sr.InternalFailure:
			step.Status = miloProto.Status_FAILURE
			step.FailureDetails = &miloProto.FailureDetails{
				Type: miloProto.FailureDetails_INFRA,
			}

		case sr.Failure:
			step.Status = miloProto.Status_FAILURE

		default:
			step.Status = miloProto.Status_SUCCESS
		}

	default:
		return fmt.Errorf("unknown swarming task state %q", sr.State)
	}

	// Compute start and finished times.
	if sr.StartedTs != "" {
		ts, err := time.Parse(SwarmingTimeLayout, sr.StartedTs)
		if err != nil {
			return fmt.Errorf("invalid task StartedTs: %s", err)
		}
		step.Started = google.NewTimestamp(ts)
	}
	if sr.CompletedTs != "" {
		ts, err := time.Parse(SwarmingTimeLayout, sr.CompletedTs)
		if err != nil {
			return fmt.Errorf("invalid task CompletedTs: %s", err)
		}
		step.Ended = google.NewTimestamp(ts)
	}

	return nil
}

func addTaskToBuild(c context.Context, server string, sr *swarming.SwarmingRpcsTaskResult, build *resp.MiloBuild) error {
	build.Summary.Label = sr.TaskId
	build.Summary.Type = resp.Recipe
	build.Summary.Source = &resp.Link{
		Label: "Task " + sr.TaskId,
		URL:   taskPageURL(server, sr.TaskId),
	}

	// Extract more swarming specific information into the properties.
	if props := taskProperties(sr); len(props.Property) > 0 {
		build.PropertyGroup = append(build.PropertyGroup, props)
	}
	if props := tagsToProperties(sr.Tags); len(props.Property) > 0 {
		build.PropertyGroup = append(build.PropertyGroup, props)
	}

	addBuilderLink(c, build, server, sr)
	addBanner(build, sr)

	// Add a link to the bot.
	if sr.BotId != "" {
		build.Summary.Bot = &resp.Link{
			Label: sr.BotId,
			URL:   botPageURL(server, sr.BotId),
		}
	}

	return nil
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

func swarmingBuildImpl(c context.Context, svc swarmingService, linkBase, taskID string) (*resp.MiloBuild, error) {
	// Fetch the data from Swarming
	fetchParams := swarmingFetchParams{
		fetchRes: true,
		fetchLog: true,
	}
	fr, err := swarmingFetch(c, svc, taskID, fetchParams)
	if err != nil {
		return nil, err
	}

	var build resp.MiloBuild
	var s *miloProto.Step
	var lds *logdog.Streams

	// Decode the data using annotee. The logdog stream returned here is assumed
	// to be consistent, which is why the following block of code are not
	// expected to ever err out.
	if fr.log != "" {
		var err error
		lds, err = streamsFromAnnotatedLog(c, fr.log)
		if err != nil {
			build.Components = []*resp.BuildComponent{{
				Type:   resp.Summary,
				Label:  "Milo annotation parser",
				Text:   []string{err.Error()},
				Status: resp.InfraFailure,
				SubLink: []*resp.Link{{
					Label: "swarming task",
					URL:   taskPageURL(svc.getHost(), taskID),
				}},
			}}
		}
	}

	if lds != nil && lds.MainStream != nil && lds.MainStream.Data != nil {
		s = lds.MainStream.Data
	} else {
		s = &miloProto.Step{}
	}

	if err := addTaskToMiloStep(c, svc.getHost(), fr.res, s); err != nil {
		return nil, err
	}
	logdog.AddLogDogToBuild(c, swarmingURLBuilder(linkBase), s, &build)

	if err := addTaskToBuild(c, svc.getHost(), fr.res, &build); err != nil {
		return nil, err
	}

	return &build, nil
}

func isMiloJob(tags []string) bool {
	for _, t := range tags {
		if t == "allow_milo:1" {
			return true
		}
	}
	return false
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

// swarmingURLBuilder is a logdog.URLBuilder that builds Milo swarming log
// links.
//
// The string value for this should be the "linkBase" parameter value supplied
// to swarmingBuildImpl.
type swarmingURLBuilder string

func (b swarmingURLBuilder) BuildLink(l *miloProto.Link) *resp.Link {
	u, err := url.Parse(string(b))
	if err != nil {
		return nil
	}

	switch t := l.Value.(type) {
	case *miloProto.Link_LogdogStream:
		ls := t.LogdogStream

		if u.Path == "" {
			u.Path = ls.Name
		} else {
			u.Path = strings.TrimSuffix(u.Path, "/") + "/" + ls.Name
		}
		link := resp.Link{
			Label: l.Label,
			URL:   u.String(),
		}
		if link.Label == "" {
			link.Label = ls.Name
		}
		return &link

	case *miloProto.Link_Url:
		return &resp.Link{
			Label: l.Label,
			URL:   t.Url,
		}

	default:
		return nil
	}
}
