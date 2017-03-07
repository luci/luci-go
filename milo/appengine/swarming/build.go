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
	"github.com/luci/luci-go/logdog/client/coordinator"
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

// logDogFetchTimeout is the amount of time to wait while fetching a LogDog
// stream before we time out the fetch.
const logDogFetchTimeout = 30 * time.Second

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

	// taskResCallback, if not nil, is a callback that will be invoked after
	// fetching the result, if fetchRes is true. It will be passed a key/value map
	// of the Swarming result's tags.
	//
	// If taskResCallback returns true, any pending log fetch will be cancelled
	// without error.
	taskResCallback func(*swarming.SwarmingRpcsTaskResult) bool
}

type swarmingFetchResult struct {
	req *swarming.SwarmingRpcsTaskRequest
	res *swarming.SwarmingRpcsTaskResult

	// log is the log data content. If no log data was fetched, this will empty.
	// If the log fetch was cancelled, this is undefined.
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

	// Special Context to enable the cancellation of log fetching.
	logsCancelled := false
	logCtx, cancelLogs := context.WithCancel(c)
	defer cancelLogs()

	err := parallel.FanOutIn(func(workC chan<- func() error) {
		if req.fetchReq {
			workC <- func() (err error) {
				fr.req, err = svc.getSwarmingRequest(c, taskID)
				return
			}
		}

		if req.fetchRes {
			workC <- func() (err error) {
				if fr.res, err = svc.getSwarmingResult(c, taskID); err == nil {
					if req.taskResCallback != nil && req.taskResCallback(fr.res) {
						logsCancelled = true
						cancelLogs()
					}
				}
				return
			}
		}

		if req.fetchLog {
			workC <- func() error {
				// Note: we're using the log Context here so we can cancel log fetch
				// explicitly.
				fr.log, logErr = svc.getTaskOutput(logCtx, taskID)
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

	// If we explicitly cancelled logs, everything is OK.
	if logErr == context.Canceled && logsCancelled {
		logErr = nil
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

func tagsToMap(tags []string) map[string]string {
	result := make(map[string]string, len(tags))
	for _, t := range tags {
		parts := strings.SplitN(t, ":", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// addBuilderLink adds a link to the buildbucket builder view.
func addBuilderLink(c context.Context, build *resp.MiloBuild, tags map[string]string) {
	bbHost := tags["buildbucket_hostname"]
	bucket := tags["buildbucket_bucket"]
	builder := tags["builder"]
	if bucket != "" && builder != "" {
		build.Summary.ParentLabel = &resp.Link{
			Label: builder,
			URL:   fmt.Sprintf("/buildbucket/%s/%s?server=%s", bucket, builder, bbHost),
		}
	}
}

// addBanner adds an OS banner derived from "os" swarming tag, if present.
func addBanner(build *resp.MiloBuild, tags map[string]string) {
	os := tags["os"]
	var ver string
	parts := strings.SplitN(os, "-", 2)
	if len(parts) == 2 {
		os = parts[0]
		ver = parts[1]
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

func addBuildsetInfo(build *resp.MiloBuild, tags map[string]string) {
	buildset := tags["buildset"]
	if !strings.HasPrefix(buildset, "patch/") {
		// Buildset isn't a patch, ignore.
		return
	}

	patchset := strings.TrimLeft(buildset, "patch/")
	// TODO(hinoka): Also support Rietveld patches.
	if strings.HasPrefix(patchset, "gerrit/") {
		gerritPatchset := strings.TrimLeft(patchset, "gerrit/")
		parts := strings.Split(gerritPatchset, "/")
		if len(parts) != 3 {
			// Not a well-formed gerrit patchset.
			return
		}
		if build.SourceStamp == nil {
			build.SourceStamp = &resp.SourceStamp{}
		}
		build.SourceStamp.Changelist = &resp.Link{
			Label: "Gerrit CL",
			URL:   fmt.Sprintf("https://%s/c/%s/%s", parts[0], parts[1], parts[2]),
		}

	}
}

func addRecipeLink(build *resp.MiloBuild, tags map[string]string) {
	name := tags["recipe_name"]
	repoURL := tags["recipe_repository"]
	revision := tags["recipe_revision"]
	if name != "" && repoURL != "" {
		if revision == "" {
			revision = "master"
		}
		// Link directly to the revision if it is a gerrit URL, otherwise just
		// display it in the name.
		if repoParse, err := url.Parse(repoURL); err == nil && strings.HasSuffix(
			repoParse.Host, ".googlesource.com") {
			repoURL += "/+/" + revision + "/"
		} else {
			if len(revision) > 8 {
				revision = revision[:8]
			}
			name += " @ " + revision
		}
		build.Summary.Recipe = &resp.Link{
			Label: name,
			URL:   repoURL,
		}
	}
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
	tags := tagsToMap(sr.Tags)

	addBuildsetInfo(build, tags)
	addBanner(build, tags)
	addBuilderLink(c, build, tags)
	addRecipeLink(build, tags)

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

// buildLoader represents the ability to load a Milo build from a Swarming task.
//
// It exists so that the internal build loading functionality can be stubbed out
// for testing.
type buildLoader struct {
	// logdogClientFunc returns a coordinator Client instance for the supplied
	// parameters.
	//
	// If nil, a production client will be generated.
	logDogClientFunc func(c context.Context, host string) (*coordinator.Client, error)
}

func (bl *buildLoader) newEmptyAnnotationStream(c context.Context, addr *types.StreamAddr) (
	*logdog.AnnotationStream, error) {

	fn := bl.logDogClientFunc
	if fn == nil {
		fn = logdog.NewClient
	}
	client, err := fn(c, addr.Host)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to create LogDog client").Err()
	}

	as := logdog.AnnotationStream{
		Client:  client,
		Project: addr.Project,
		Path:    addr.Path,
	}
	if err := as.Normalize(); err != nil {
		return nil, errors.Annotate(err).Reason("failed to normalize annotation stream parameters").Err()
	}

	return &as, nil
}

func (bl *buildLoader) swarmingBuildImpl(c context.Context, svc swarmingService, linkBase, taskID string) (*resp.MiloBuild, error) {
	// Fetch the data from Swarming
	var logDogStreamAddr *types.StreamAddr

	fetchParams := swarmingFetchParams{
		fetchRes: true,
		fetchLog: true,

		// Cancel if LogDog annotation stream parameters are present in the tag set.
		taskResCallback: func(res *swarming.SwarmingRpcsTaskResult) (cancelLogs bool) {
			// If the build hasn't started yet, then there is no LogDog log stream to
			// render.
			switch res.State {
			case TaskPending, TaskExpired:
				return false

			case TaskCanceled:
				// If the task wasn't created, then it wasn't started.
				if res.CreatedTs == "" {
					return false
				}
			}

			// The task started ... is it using LogDog for logging?
			tags := swarmingTags(res.Tags)

			var err error
			if logDogStreamAddr, err = resolveLogDogStreamAddrFromTags(tags, res.TaskId, res.TryNumber); err != nil {
				logging.WithError(err).Debugf(c, "Not using LogDog annotation stream.")
				return false
			}
			return true
		},
	}
	fr, err := swarmingFetch(c, svc, taskID, fetchParams)
	if err != nil {
		return nil, err
	}

	var build resp.MiloBuild
	var s *miloProto.Step
	var lds *logdog.Streams
	var ub logdog.URLBuilder

	// Load the build from the available data.
	//
	// If the Swarming task explicitly specifies its log location, we prefer that.
	// As a fallback, we will try and parse the Swarming task's output for
	// annotations.
	switch {
	case logDogStreamAddr != nil:
		logging.Infof(c, "Loading build from LogDog stream at: %s", logDogStreamAddr)

		// If the LogDog stream is available, load the step from that.
		as, err := bl.newEmptyAnnotationStream(c, logDogStreamAddr)
		if err != nil {
			return nil, errors.Annotate(err).Reason("failed to create LogDog annotation stream").Err()
		}

		prefix, _ := logDogStreamAddr.Path.Split()
		ub = &logdog.ViewerURLBuilder{
			Host:    logDogStreamAddr.Host,
			Prefix:  prefix,
			Project: logDogStreamAddr.Project,
		}

		if s, err = as.Fetch(c); err != nil {
			switch errors.Unwrap(err) {
			case coordinator.ErrNoSuchStream:
				logging.WithError(err).Errorf(c, "User cannot access stream.")
				build.Components = append(build.Components, infoComponent(resp.Running,
					"Waiting...", "waiting for annotation stream"))

			case coordinator.ErrNoAccess:
				logging.WithError(err).Errorf(c, "User cannot access stream.")
				build.Components = append(build.Components, infoComponent(resp.Failure,
					"No Access", "no access to annotation stream"))

			default:
				logging.WithError(err).Errorf(c, "Failed to load LogDog annotation stream.")
				build.Components = append(build.Components, infoComponent(resp.InfraFailure,
					"Error", "failed to load annotation stream"))
			}
		}

	case fr.log != "":
		// Decode the data using annotee. The logdog stream returned here is assumed
		// to be consistent, which is why the following block of code are not
		// expected to ever err out.
		var err error
		lds, err = streamsFromAnnotatedLog(c, fr.log)
		if err != nil {
			comp := infoComponent(resp.InfraFailure, "Milo annotation parser", err.Error())
			comp.SubLink = append(comp.SubLink, resp.LinkSet{&resp.Link{
				Label: "swarming task",
				URL:   taskPageURL(svc.getHost(), taskID),
			}})
			build.Components = append(build.Components, comp)
		}

		if lds != nil && lds.MainStream != nil && lds.MainStream.Data != nil {
			s = lds.MainStream.Data
		}
		ub = swarmingURLBuilder(linkBase)

	default:
		s = &miloProto.Step{}
		ub = swarmingURLBuilder(linkBase)
	}

	if s != nil {
		if err := addTaskToMiloStep(c, svc.getHost(), fr.res, s); err != nil {
			return nil, err
		}
		logdog.AddLogDogToBuild(c, ub, s, &build)
	}

	if err := addTaskToBuild(c, svc.getHost(), fr.res, &build); err != nil {
		return nil, err
	}

	return &build, nil
}

func infoComponent(st resp.Status, label, text string) *resp.BuildComponent {
	return &resp.BuildComponent{
		Type:   resp.Summary,
		Label:  label,
		Text:   []string{text},
		Status: st,
	}
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

func swarmingTags(v []string) map[string]string {
	res := make(map[string]string, len(v))
	for _, tag := range v {
		var value string
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			value = parts[1]
		}
		res[parts[0]] = value
	}
	return res
}
