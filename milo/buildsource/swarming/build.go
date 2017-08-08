// Copyright 2015 The LUCI Authors.
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

package swarming

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/logdog/client/annotee"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
)

// errNotMiloJob is returned if a Swarming task is fetched that does not self-
// identify as a Milo job.
var errNotMiloJob = errors.New("Not a Milo Job or access denied", common.CodeNoAccess)

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

func getSwarmingClient(c context.Context, host string) (*swarming.Service, error) {
	c, _ = context.WithTimeout(c, 60*time.Second)
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	sc, err := swarming.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", host)
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

func NewProdService(c context.Context, host string) (*prodSwarmingService, error) {
	client, err := getSwarmingClient(c, host)
	if err != nil {
		return nil, err
	}
	return &prodSwarmingService{
		host:   host,
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

	// Current ACL implementation:
	// If allow_milo:1 is present, it is a public job.  Don't bother with ACL check.
	// If it is not present, check the luci_project tag, and see if user is allowed
	// to access said project.
	switch {
	case req.fetchReq:
		if !isAllowed(c, fr.req.Tags) {
			return nil, errNotMiloJob
		}

	case req.fetchRes:
		if !isAllowed(c, fr.res.Tags) {
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
	bucket := tags["buildbucket_bucket"]
	builder := tags["builder"]
	if bucket != "" && builder != "" {
		build.Summary.ParentLabel = resp.NewLink(
			builder, fmt.Sprintf("/buildbucket/%s/%s", bucket, builder))
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
		build.SourceStamp.Changelist = resp.NewLink(
			"Gerrit CL", fmt.Sprintf("https://%s/c/%s/%s", parts[0], parts[1], parts[2]))

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
		build.Summary.Recipe = resp.NewLink(name, repoURL)
	}
}

func addTaskToBuild(c context.Context, server string, sr *swarming.SwarmingRpcsTaskResult, build *resp.MiloBuild) error {
	build.Summary.Label = sr.TaskId
	build.Summary.Type = resp.Recipe
	build.Summary.Source = resp.NewLink("Task "+sr.TaskId, taskPageURL(server, sr.TaskId))

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
		build.Summary.Bot = resp.NewLink(sr.BotId, botPageURL(server, sr.BotId))
	}

	return nil
}

// streamsFromAnnotatedLog takes in an annotated log and returns a fully
// populated set of logdog streams
func streamsFromAnnotatedLog(ctx context.Context, log string) (*rawpresentation.Streams, error) {
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

// BuildLoader represents the ability to load a Milo build from a Swarming task.
//
// It exists so that the internal build loading functionality can be stubbed out
// for testing.
type BuildLoader struct {
	// logdogClientFunc returns a coordinator Client instance for the supplied
	// parameters.
	//
	// If nil, a production client will be generated.
	logDogClientFunc func(c context.Context, host string) (*coordinator.Client, error)
}

func (bl *BuildLoader) newEmptyAnnotationStream(c context.Context, addr *types.StreamAddr) (
	*rawpresentation.AnnotationStream, error) {

	fn := bl.logDogClientFunc
	if fn == nil {
		fn = rawpresentation.NewClient
	}
	client, err := fn(c, addr.Host)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create LogDog client").Err()
	}

	as := rawpresentation.AnnotationStream{
		Client:  client,
		Project: addr.Project,
		Path:    addr.Path,
	}
	if err := as.Normalize(); err != nil {
		return nil, errors.Annotate(err, "failed to normalize annotation stream parameters").Err()
	}

	return &as, nil
}

// failedToStart is called in the case where logdog-only mode is on but the
// stream doesn't exist and the swarming job is complete.  It modifies the build
// to add information that would've otherwise been in the annotation stream.
func failedToStart(c context.Context, build *resp.MiloBuild, res *swarming.SwarmingRpcsTaskResult, host string) error {
	var err error
	build.Summary.Status = model.InfraFailure
	build.Summary.Started, err = time.Parse(SwarmingTimeLayout, res.StartedTs)
	if err != nil {
		return err
	}
	build.Summary.Finished, err = time.Parse(SwarmingTimeLayout, res.CompletedTs)
	if err != nil {
		return err
	}
	build.Summary.Duration = build.Summary.Finished.Sub(build.Summary.Started)
	infoComp := infoComponent(model.InfraFailure,
		"LogDog stream not found", "Job likely failed to start.")
	infoComp.Started = build.Summary.Started
	infoComp.Finished = build.Summary.Finished
	infoComp.Duration = build.Summary.Duration
	infoComp.Verbosity = resp.Interesting
	build.Components = append(build.Components, infoComp)
	return addTaskToBuild(c, host, res, build)
}

func (bl *BuildLoader) SwarmingBuildImpl(c context.Context, svc swarmingService, taskID string) (*resp.MiloBuild, error) {
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
	var lds *rawpresentation.Streams
	var ub rawpresentation.URLBuilder

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
			return nil, errors.Annotate(err, "failed to create LogDog annotation stream").Err()
		}

		prefix, _ := logDogStreamAddr.Path.Split()
		ub = &rawpresentation.ViewerURLBuilder{
			Host:    logDogStreamAddr.Host,
			Prefix:  prefix,
			Project: logDogStreamAddr.Project,
		}

		if s, err = as.Fetch(c); err != nil {
			switch errors.Unwrap(err) {
			case coordinator.ErrNoSuchStream:
				// The stream was not found.  This could be due to one of two things:
				// 1. The step just started and we're just waiting for the logs
				// to propogage to logdog.
				// 2. The bootsrap on the client failed, and never sent data to logdog.
				// This would be evident because the swarming result would be a failure.
				if fr.res.State == TaskCompleted {
					err = failedToStart(c, &build, fr.res, svc.getHost())
					return &build, err
				}
				logging.WithError(err).Errorf(c, "User cannot access stream.")
				build.Components = append(build.Components, infoComponent(model.Running,
					"Waiting...", "waiting for annotation stream"))

			case coordinator.ErrNoAccess:
				logging.WithError(err).Errorf(c, "User cannot access stream.")
				build.Components = append(build.Components, infoComponent(model.Failure,
					"No Access", "no access to annotation stream"))

			default:
				logging.WithError(err).Errorf(c, "Failed to load LogDog annotation stream.")
				build.Components = append(build.Components, infoComponent(model.InfraFailure,
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
			comp := infoComponent(model.InfraFailure, "Milo annotation parser", err.Error())
			comp.SubLink = append(comp.SubLink, resp.LinkSet{
				resp.NewLink("swarming task", taskPageURL(svc.getHost(), taskID)),
			})
			build.Components = append(build.Components, comp)
		}

		if lds != nil && lds.MainStream != nil && lds.MainStream.Data != nil {
			s = lds.MainStream.Data
		}
		ub = swarmingURLBuilder(taskID)

	default:
		s = &miloProto.Step{}
		ub = swarmingURLBuilder(taskID)
	}

	if s != nil {
		if err := addTaskToMiloStep(c, svc.getHost(), fr.res, s); err != nil {
			return nil, err
		}
		rawpresentation.AddLogDogToBuild(c, ub, s, &build)
	}

	if err := addTaskToBuild(c, svc.getHost(), fr.res, &build); err != nil {
		return nil, err
	}

	return &build, nil
}

func infoComponent(st model.Status, label, text string) *resp.BuildComponent {
	return &resp.BuildComponent{
		Type:   resp.Summary,
		Label:  label,
		Text:   []string{text},
		Status: st,
	}
}

// isAllowed checks if:
// 1. allow_milo:1 is present.  If so, it's a public job.
// 2. luci_project is present, and if the logged in user has access to that project.
func isAllowed(c context.Context, tags []string) bool {
	for _, t := range tags {
		if t == "allow_milo:1" {
			return true
		}
	}
	for _, t := range tags {
		if strings.HasPrefix(t, "luci_project:") {
			sp := strings.SplitN(t, ":", 2)
			if len(sp) != 2 {
				return false
			}
			logging.Debugf(c, "Checking if user has access to %s", sp[1])
			// sp[1] is the project ID.
			allowed, err := common.IsAllowed(c, sp[1])
			if err != nil {
				logging.WithError(err).Errorf(c, "could not perform acl check")
				return false
			}
			return allowed
		}
	}
	return false
}

// taskPageURL returns a URL to a human-consumable page of a swarming task.
// Supports server aliases.
func taskPageURL(swarmingHostname, taskID string) string {
	return fmt.Sprintf("https://%s/task?id=%s&show_raw=1&wide_logs=true", swarmingHostname, taskID)
}

// botPageURL returns a URL to a human-consumable page of a swarming bot.
// Supports server aliases.
func botPageURL(swarmingHostname, botID string) string {
	return fmt.Sprintf("https://%s/restricted/bot/%s", swarmingHostname, botID)
}

// URLBase is the routing prefix for swarming endpoints. It's here so that it
// can be a constant between the swarmingURLBuilder and the frontend.
const URLBase = "/swarming/task"

// swarmingURLBuilder is a logdog.URLBuilder that builds Milo swarming log
// links.
//
// It should be the swarming task id.
type swarmingURLBuilder string

func (b swarmingURLBuilder) BuildLink(l *miloProto.Link) *resp.Link {
	switch t := l.Value.(type) {
	case *miloProto.Link_LogdogStream:
		ls := t.LogdogStream

		link := resp.NewLink(l.Label, fmt.Sprintf("%s/%s/%s", URLBase, b, ls.Name))
		if link.Label == "" {
			link.Label = ls.Name
		}
		return link

	case *miloProto.Link_Url:
		return resp.NewLink(l.Label, t.Url)

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

// BuildID is swarming's notion of a Build. See buildsource.ID.
type BuildID struct {
	// (Required) The Swarming TaskID.
	TaskID string

	// (Optional) The Swarming host. If empty, will use the
	// milo-instance-configured swarming host.
	Host string
}

// getSwarmingHost parses the swarming hostname out of the context.  If
// none is specified, get the default swarming host out of the global
// configs.
func (b *BuildID) getSwarmingHost(c context.Context) (server string, err error) {
	server = b.Host
	settings := common.GetSettings(c)
	if settings.Swarming == nil {
		err := errors.New("swarming not in settings")
		logging.WithError(err).Errorf(c, "Go configure swarming in the settings page.")
		return "", err
	}
	if server == "" || server == settings.Swarming.DefaultHost {
		return settings.Swarming.DefaultHost, nil
	}
	// If it is specified, validate the hostname.
	for _, hostname := range settings.Swarming.AllowedHosts {
		if server == hostname {
			return server, nil
		}
	}
	return "", errors.New("unknown swarming host", common.CodeParameterError)
}

func (b *BuildID) validate() error {
	if b.TaskID == "" {
		return errors.New("no swarming task id", common.CodeParameterError)
	}
	return nil
}

// Get implements buildsource.ID
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	hostname, err := b.getSwarmingHost(c)
	if err != nil {
		return nil, err
	}
	sf, err := NewProdService(c, hostname)
	if err != nil {
		return nil, err
	}

	return (&BuildLoader{}).SwarmingBuildImpl(c, sf, b.TaskID)
}

// GetLog implements buildsource.ID
func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	if err = b.validate(); err != nil {
		return
	}
	if logname == "" {
		err = errors.New("no log name", common.CodeParameterError)
		return
	}

	hostname, err := b.getSwarmingHost(c)
	if err != nil {
		return
	}

	sf, err := NewProdService(c, hostname)
	if err != nil {
		return
	}

	return swarmingBuildLogImpl(c, sf, b.TaskID, logname)
}
