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
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luciexe/legacy/annotee"
	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"
	"go.chromium.org/luci/server/auth"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/internal/config"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
)

// swarmingService is an interface that fetches data from Swarming.
//
// In production, this is fetched from a Swarming host. For testing, this can
// be replaced with a mock.
type swarmingService interface {
	GetHost() string
	GetSwarmingResult(c context.Context, taskID string) (*swarmingpb.TaskResultResponse, error)
	GetSwarmingRequest(c context.Context, taskID string) (*swarmingpb.TaskRequestResponse, error)
	GetTaskOutput(c context.Context, taskID string) (string, error)
}

// ErrNotMiloJob is returned if a Swarming task is fetched that does not self-
// identify as a Milo job.
var ErrNotMiloJob = grpcutil.PermissionDeniedTag.Apply(errors.New("Not a Milo Job or access denied"))

func getSwarmingClient(c context.Context, host string) (swarmingpb.TasksClient, error) {
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return swarmingpb.NewTasksClient(&prpc.Client{
		C:    &http.Client{Transport: t},
		Host: host,
	}), nil
}

type prodSwarmingService struct {
	host   string
	client swarmingpb.TasksClient
}

func newProdService(c context.Context, host string) (*prodSwarmingService, error) {
	host, err := getSwarmingHost(c, host)
	if err != nil {
		return nil, err
	}
	client, err := getSwarmingClient(c, host)
	if err != nil {
		return nil, err
	}
	return &prodSwarmingService{
		host:   host,
		client: client,
	}, nil
}

func (svc *prodSwarmingService) GetHost() string { return svc.host }

func (svc *prodSwarmingService) GetSwarmingResult(c context.Context, taskID string) (*swarmingpb.TaskResultResponse, error) {
	return svc.client.GetResult(c, &swarmingpb.TaskIdWithPerfRequest{
		TaskId: taskID,
	})
}

func (svc *prodSwarmingService) GetTaskOutput(c context.Context, taskID string) (string, error) {
	stdout, err := svc.client.GetStdout(c, &swarmingpb.TaskIdWithOffsetRequest{
		TaskId: taskID,
	})
	if err != nil {
		return "", err
	}
	return string(stdout.Output), nil
}

func (svc *prodSwarmingService) GetSwarmingRequest(c context.Context, taskID string) (*swarmingpb.TaskRequestResponse, error) {
	return svc.client.GetRequest(c, &swarmingpb.TaskIdRequest{
		TaskId: taskID,
	})
}

type swarmingFetchParams struct {
	fetchLog bool

	// taskResCallback, if not nil, is a callback that will be invoked after
	// fetching the result.
	//
	// If taskResCallback returns true, any pending log fetch will be canceled
	// without error.
	taskResCallback func(*swarmingpb.TaskResultResponse) bool
}

type swarmingFetchResult struct {
	res *swarmingpb.TaskResultResponse

	// log is the log data content. If no log data was fetched, this will empty.
	// If the log fetch was canceled, this is undefined.
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
	logsCanceled := false
	logCtx, cancelLogs := context.WithCancel(c)
	defer cancelLogs()

	err := parallel.FanOutIn(func(workC chan<- func() error) {
		workC <- func() (err error) {
			if fr.res, err = svc.GetSwarmingResult(c, taskID); err == nil {
				if req.taskResCallback != nil && req.taskResCallback(fr.res) {
					logsCanceled = true
					cancelLogs()
				}
			}
			return
		}

		if req.fetchLog {
			workC <- func() error {
				// Note: we're using the log Context here so we can cancel log fetch
				// explicitly.
				fr.log, logErr = svc.GetTaskOutput(logCtx, taskID)
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
	if !isAllowed(c, fr.res.Tags) {
		return nil, ErrNotMiloJob
	}

	if logErr != nil {
		switch fr.res.State {
		case swarmingpb.TaskState_COMPLETED,
			swarmingpb.TaskState_RUNNING,
			swarmingpb.TaskState_CANCELED,
			swarmingpb.TaskState_KILLED,
			swarmingpb.TaskState_NO_RESOURCE:
		default:
			//  Ignore log errors if the task might be pending, timed out, expired, etc.
			if err != nil {
				fr.log = ""
				logErr = nil
			}
		}
	}

	// If we explicitly canceled logs, everything is OK.
	if logErr == context.Canceled && logsCanceled {
		logErr = nil
	}
	return &fr, logErr
}

func taskProperties(sr *swarmingpb.TaskResultResponse) *ui.PropertyGroup {
	props := &ui.PropertyGroup{GroupName: "Swarming"}
	if len(sr.CostsUsd) == 1 {
		props.Property = append(props.Property, &ui.Property{
			Key:   "Cost of job (USD)",
			Value: fmt.Sprintf("$%.2f", sr.CostsUsd[0]),
		})
	}
	if sr.State == swarmingpb.TaskState_COMPLETED || sr.State == swarmingpb.TaskState_TIMED_OUT {
		props.Property = append(props.Property, &ui.Property{
			Key:   "Exit Code",
			Value: fmt.Sprintf("%d", sr.ExitCode),
		})
	}
	return props
}

// addBuilderLink adds a link to the buildbucket builder view.
func addBuilderLink(c context.Context, build *ui.MiloBuildLegacy, tags strpair.Map) {
	bucket := tags.Get("buildbucket_bucket")
	builder := tags.Get("builder")
	project := tags.Get("luci_project")
	if bucket != "" && builder != "" {
		builderParts := strings.Split(builder, "/")
		builder = builderParts[len(builderParts)-1]
		build.Summary.ParentLabel = ui.NewLink(
			builder, fmt.Sprintf("/p/%s/builders/%s/%s", project, bucket, builder),
			fmt.Sprintf("buildbucket builder %s on bucket %s", builder, bucket))
	}
}

// AddBanner adds an OS banner derived from "os" swarming tag, if present.
func AddBanner(build *ui.MiloBuildLegacy, tags strpair.Map) {
	os := tags.Get("os")
	parts := strings.SplitN(os, "-", 2)
	var ver string
	if len(parts) == 2 {
		os = parts[0]
		ver = parts[1]
	}

	var base ui.LogoBase
	switch os {
	case "Ubuntu":
		base = ui.Ubuntu
	case "Windows":
		base = ui.Windows
	case "Mac":
		base = ui.OSX
	case "Android":
		base = ui.Android
	default:
		return
	}
	build.Summary.Banner = &ui.LogoBanner{
		OS: []ui.Logo{{
			LogoBase: base,
			Subtitle: ver,
			Count:    1,
		}},
	}
}

// addTaskToMiloStep augments a Milo Annotation Protobuf with state from the
// Swarming task.
func addTaskToMiloStep(c context.Context, host string, sr *swarmingpb.TaskResultResponse, step *annopb.Step) error {
	step.Link = &annopb.AnnotationLink{
		Label: "Task " + sr.TaskId,
		Value: &annopb.AnnotationLink_Url{
			Url: TaskPageURL(host, sr.TaskId).String(),
		},
	}

	switch sr.State {
	case swarmingpb.TaskState_RUNNING:
		step.Status = annopb.Status_RUNNING

	case swarmingpb.TaskState_PENDING:
		step.Status = annopb.Status_PENDING

	case swarmingpb.TaskState_EXPIRED,
		swarmingpb.TaskState_TIMED_OUT,
		swarmingpb.TaskState_BOT_DIED,
		swarmingpb.TaskState_CLIENT_ERROR:
		step.Status = annopb.Status_FAILURE

		switch sr.State {
		case swarmingpb.TaskState_EXPIRED:
			step.FailureDetails = &annopb.FailureDetails{
				Type: annopb.FailureDetails_EXPIRED,
				Text: "Task expired",
			}
		case swarmingpb.TaskState_TIMED_OUT:
			step.FailureDetails = &annopb.FailureDetails{
				Type: annopb.FailureDetails_INFRA,
				Text: "Task timed out",
			}
		case swarmingpb.TaskState_BOT_DIED:
			step.FailureDetails = &annopb.FailureDetails{
				Type: annopb.FailureDetails_INFRA,
				Text: "Bot died",
			}
		case swarmingpb.TaskState_CLIENT_ERROR:
			step.FailureDetails = &annopb.FailureDetails{
				Type: annopb.FailureDetails_INFRA,
				Text: "Client error",
			}
		}

	case swarmingpb.TaskState_CANCELED, swarmingpb.TaskState_KILLED:
		// Canceled build is user action, so it is not an infra failure.
		step.Status = annopb.Status_FAILURE
		step.FailureDetails = &annopb.FailureDetails{
			Type: annopb.FailureDetails_CANCELLED,
			Text: "Task canceled by user",
		}

	case swarmingpb.TaskState_NO_RESOURCE:
		step.Status = annopb.Status_FAILURE
		step.FailureDetails = &annopb.FailureDetails{
			Type: annopb.FailureDetails_EXPIRED,
			Text: "No resource available on Swarming",
		}

	case swarmingpb.TaskState_COMPLETED:

		switch {
		case sr.InternalFailure:
			step.Status = annopb.Status_FAILURE
			step.FailureDetails = &annopb.FailureDetails{
				Type: annopb.FailureDetails_INFRA,
			}

		case sr.Failure:
			step.Status = annopb.Status_FAILURE

		default:
			step.Status = annopb.Status_SUCCESS
		}

	default:
		return fmt.Errorf("unknown swarming task state %q", sr.State)
	}

	step.Started = sr.StartedTs
	step.Ended = sr.CompletedTs
	return nil
}

func addBuildsetInfo(build *ui.MiloBuildLegacy, tags strpair.Map) {
	for _, bs := range tags["buildset"] {
		if cl, ok := protoutil.ParseBuildSet(bs).(*buildbucketpb.GerritChange); ok {
			if build.Trigger == nil {
				build.Trigger = &ui.Trigger{}
			}
			build.Trigger.Changelist = ui.NewPatchLink(cl)
			break
		}
	}
}

var regexRepoFromRecipeBundle = regexp.MustCompile(`/[^/]+\.googlesource\.com/.+$`)

// AddRecipeLink adds links to the recipe to the build.
func AddRecipeLink(build *ui.MiloBuildLegacy, tags strpair.Map) {
	name := tags.Get("recipe_name")
	repoURL := tags.Get("recipe_repository")
	switch {
	case name == "":
		return
	case repoURL == "":
		// Was recipe_bundler-created CIPD package used?
		repoURL = regexRepoFromRecipeBundle.FindString(tags.Get("recipe_package"))
		if repoURL == "" {
			return
		}
		// note that regex match will start with a slash, e.g.,
		// "/chromium.googlesource.com/infra/infra"
		repoURL = "https:/" + repoURL // make it valid URL.
	}

	// We don't know location of recipes within the repo and getting that
	// information is not trivial, so use code search, which is precise enough.
	// TODO(nodir): load location from infra/config/recipes.cfg of the
	// recipe_repository.
	csHost := "cs.chromium.org"
	repoURLParsed, _ := url.Parse(repoURL)
	if repoURLParsed != nil && strings.Contains(repoURLParsed.Host, "internal") {
		csHost = "cs.corp.google.com"
	}
	recipeURL := fmt.Sprintf("https://%s/search/?q=file:recipes/%s.py", csHost, name)
	build.Summary.Recipe = ui.NewLink(name, recipeURL, fmt.Sprintf("recipe %s", name))
}

// AddProjectInfo adds the luci_project swarming tag to the build.
func AddProjectInfo(build *ui.MiloBuildLegacy, tags strpair.Map) {
	if proj := tags.Get("luci_project"); proj != "" {
		if build.Trigger == nil {
			build.Trigger = &ui.Trigger{}
		}
		build.Trigger.Project = proj
	}
}

// addPendingTiming adds pending timing information to the build.
func addPendingTiming(c context.Context, build *ui.MiloBuildLegacy, sr *swarmingpb.TaskResultResponse) {
	if sr.CreatedTs != nil {
		build.Summary.PendingTime = ui.NewInterval(c, sr.CreatedTs.AsTime(), build.Summary.ExecutionTime.Started)
	}
}

func addTaskToBuild(c context.Context, host string, sr *swarmingpb.TaskResultResponse, build *ui.MiloBuildLegacy) error {
	build.Summary.Label = ui.NewEmptyLink(sr.TaskId)
	build.Summary.Type = ui.Recipe
	build.Summary.Source = ui.NewLink(
		"Task "+sr.TaskId, TaskPageURL(host, sr.TaskId).String(),
		fmt.Sprintf("swarming task %s", sr.TaskId))

	// Extract more swarming specific information into the properties.
	if props := taskProperties(sr); len(props.Property) > 0 {
		build.PropertyGroup = append(build.PropertyGroup, props)
	}
	tags := strpair.ParseMap(sr.Tags)

	addBuildsetInfo(build, tags)
	AddBanner(build, tags)
	addBuilderLink(c, build, tags)
	AddRecipeLink(build, tags)
	AddProjectInfo(build, tags)
	addPendingTiming(c, build, sr)

	// Add a link to the bot.
	if sr.BotId != "" {
		build.Summary.Bot = ui.NewLink(sr.BotId, botPageURL(host, sr.BotId),
			fmt.Sprintf("swarming bot %s", sr.BotId))
	}

	return nil
}

// streamsFromAnnotatedLog takes in an annotated log and returns a fully
// populated set of logdog streams
func streamsFromAnnotatedLog(ctx context.Context, log string) (*rawpresentation.Streams, error) {
	scFake, c := streamclient.NewUnregisteredFake("")
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
	return parseAnnotations(scFake)
}

// failedToStart is called in the case where logdog-only mode is on but the
// stream doesn't exist and the swarming job is complete.  It modifies the build
// to add information that would've otherwise been in the annotation stream.
func failedToStart(c context.Context, build *ui.MiloBuildLegacy, res *swarmingpb.TaskResultResponse, host string) error {
	build.Summary.Status = milostatus.InfraFailure
	if res.StartedTs == nil {
		return errors.New("no started time in the Swarming task")
	}
	if res.CompletedTs == nil {
		return errors.New("no completed time in the Swarming task")
	}
	build.Summary.ExecutionTime = ui.NewInterval(c, res.StartedTs.AsTime(), res.CompletedTs.AsTime())
	infoComp := infoComponent(milostatus.InfraFailure,
		"LogDog stream not found", "Job likely failed to start.")
	infoComp.ExecutionTime = build.Summary.ExecutionTime
	build.Components = append(build.Components, infoComp)
	return addTaskToBuild(c, host, res, build)
}

// swarmingFetchMaybeLogs fetches the swarming task result.  It also fetches
// the log iff the task is not a logdog enabled task.
func swarmingFetchMaybeLogs(c context.Context, svc swarmingService, taskID string) (
	*swarmingFetchResult, *types.StreamAddr, error) {
	// Fetch the data from Swarming
	var logDogStreamAddr *types.StreamAddr

	fetchParams := swarmingFetchParams{
		fetchLog: true,

		// Cancel if LogDog annotation stream parameters are present in the tag set.
		taskResCallback: func(res *swarmingpb.TaskResultResponse) (cancelLogs bool) {
			// If the build hasn't started yet, then there is no LogDog log stream to
			// render.
			switch res.State {
			case swarmingpb.TaskState_PENDING, swarmingpb.TaskState_EXPIRED:
				return false

			case swarmingpb.TaskState_CANCELED, swarmingpb.TaskState_KILLED:
				// If the task wasn't created, then it wasn't started.
				if res.CreatedTs == nil {
					return false
				}
			}

			// The task started ... is it using LogDog for logging?
			tags := swarmingTags(res.Tags)

			var err error
			if logDogStreamAddr, err = resolveLogDogStreamAddrFromTags(tags); err != nil {
				logging.WithError(err).Debugf(c, "Not using LogDog annotation stream.")
				return false
			}
			return true
		},
	}
	fr, err := swarmingFetch(c, svc, taskID, fetchParams)
	return fr, logDogStreamAddr, err
}

// resolveLogDogStreamAddrFromTags returns a configured AnnotationStream given
// the tags swarming task's tags.
func resolveLogDogStreamAddrFromTags(tags map[string]string) (*types.StreamAddr, error) {
	// If we don't have a LUCI project, abort.
	luciProject, logLocation := tags["luci_project"], tags["log_location"]
	switch {
	case luciProject == "":
		return nil, errors.New("no 'luci_project' tag")
	case logLocation == "":
		return nil, errors.New("no 'log_location' tag")
	}

	addr, err := types.ParseURL(logLocation)
	if err != nil {
		return nil, errors.Fmt("could not parse LogDog stream from location: %w", err)
	}

	// The LogDog stream's project should match the LUCI project.
	if string(addr.Project) != luciProject {
		return nil, errors.Fmt("stream project %q doesn't match LUCI project %q", addr.Project, luciProject)
	}

	return addr, nil
}

// buildFromLogs returns a milo build from just the swarming log and result data.
// TODO(hinoka): Remove this once skia moves logging to logdog/kitchen.
func buildFromLogs(c context.Context, taskURL *url.URL, fr *swarmingFetchResult) (*ui.MiloBuildLegacy, error) {
	var build ui.MiloBuildLegacy
	var step *annopb.Step

	// Decode the data using annotee. The logdog stream returned here is assumed
	// to be consistent, which is why the following block of code are not
	// expected to ever err out.
	if fr.log != "" {
		lds, err := streamsFromAnnotatedLog(c, fr.log)
		if err != nil {
			comp := infoComponent(milostatus.InfraFailure, "Milo annotation parser", err.Error())
			comp.SubLink = append(comp.SubLink, ui.LinkSet{
				ui.NewLink("swarming task", taskURL.String(), ""),
			})
			build.Components = append(build.Components, comp)
		} else if lds.MainStream != nil {
			step = proto.Clone(lds.MainStream.Data).(*annopb.Step)
		}
	}

	if err := addTaskToMiloStep(c, taskURL.Host, fr.res, step); err != nil {
		return nil, err
	}

	// Log links are built relative to swarming URLs
	id := taskURL.Query().Get("id")
	ub := swarmingURLBuilder(id)
	rawpresentation.AddLogDogToBuild(c, ub, step, &build)

	addFailureSummary(&build)

	err := addTaskToBuild(c, taskURL.Host, fr.res, &build)
	return &build, err
}

// addFailureSummary adds failure summary information to the main status,
// derivied from individual steps.
func addFailureSummary(b *ui.MiloBuildLegacy) {
	for _, comp := range b.Components {
		// Add interesting information into the main summary text.
		if comp.Status != milostatus.Success {
			b.Summary.Text = append(
				b.Summary.Text, fmt.Sprintf("%s %s", comp.Status, comp.Label))
		}
	}
}

// SwarmingBuildImpl fetches data from Swarming and LogDog and produces a resp.MiloBuildLegacy
// representation of a build state given a Swarming TaskID.
func SwarmingBuildImpl(c context.Context, svc swarmingService, taskID string) (*ui.MiloBuildLegacy, error) {
	// First, get the task result from swarming, and maybe the logs.
	fr, logDogStreamAddr, err := swarmingFetchMaybeLogs(c, svc, taskID)
	if err != nil {
		return nil, err
	}
	swarmingResult := fr.res

	// Legacy codepath - Annotations are encoded in the swarming log instead of LogDog.
	// TODO(hinoka): Remove this once skia moves logging to logdog/kitchen.
	if logDogStreamAddr == nil {
		taskURL := TaskPageURL(svc.GetHost(), taskID)
		return buildFromLogs(c, taskURL, fr)
	}

	// Create an empty build here first because we might want to add some
	// system-level messages.
	var build ui.MiloBuildLegacy

	// Load the build from the LogDog service.  For known classes of errors, add
	// steps in the build presentation to explain what may be going on.
	step, err := rawpresentation.ReadAnnotations(c, logDogStreamAddr)
	switch errors.Unwrap(err) {
	case coordinator.ErrNoSuchStream:
		// The stream was not found.  This could be due to one of two things:
		// 1. The step just started and we're just waiting for the logs
		// to propogage to logdog.
		// 2. The bootstrap on the client failed, and never sent data to logdog.
		// This would be evident because the swarming result would be a failure.
		if swarmingResult.State == swarmingpb.TaskState_COMPLETED {
			err = failedToStart(c, &build, swarmingResult, svc.GetHost())
			return &build, err
		}
		logging.WithError(err).Errorf(c, "User cannot access stream.")
		build.Components = append(build.Components, infoComponent(milostatus.Running,
			"Waiting...", "waiting for annotation stream"))

	case coordinator.ErrNoAccess:
		logging.WithError(err).Errorf(c, "User cannot access stream.")
		build.Components = append(build.Components, infoComponent(milostatus.Failure,
			"No Access", "no access to annotation stream"))
	case nil:
		// continue

	default:
		logging.WithError(err).Errorf(c, "Failed to load LogDog annotation stream.")
		build.Components = append(build.Components, infoComponent(milostatus.InfraFailure,
			"Error", "failed to load annotation stream: "+err.Error()))
	}

	// Skip these steps if the LogDog stream doesn't exist.
	// i.e. when the stream isn't ready yet, or errored out.
	if step != nil {
		// Milo Step Proto += Swarming Result Data
		if err := addTaskToMiloStep(c, svc.GetHost(), swarmingResult, step); err != nil {
			return nil, err
		}
		// Log links are linked directly to the logdog service.  This is used when
		// converting proto step data to resp build structs
		ub := rawpresentation.NewURLBuilder(logDogStreamAddr)
		rawpresentation.AddLogDogToBuild(c, ub, step, &build)
	}
	addFailureSummary(&build)

	// Milo Resp Build += Swarming Result Data
	// This is done for things in resp but not in step like the banner, buildset,
	// recipe link, bot info, title, etc.
	err = addTaskToBuild(c, svc.GetHost(), swarmingResult, &build)
	return &build, err
}

// infoComponent is a helper function to return a resp build step with the
// given status, label, and step text.
func infoComponent(st milostatus.Status, label, text string) *ui.BuildComponent {
	return &ui.BuildComponent{
		Type:   ui.Summary,
		Label:  ui.NewEmptyLink(label),
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
			allowed, err := projectconfig.IsAllowed(c, sp[1])
			if err != nil {
				logging.WithError(err).Errorf(c, "could not perform acl check")
				return false
			}
			return allowed
		}
	}
	return false
}

// TaskPageURL returns a URL to a human-consumable page of a swarming task.
// Supports host aliases.
func TaskPageURL(swarmingHostname, taskID string) *url.URL {
	val := url.Values{}
	val.Set("id", taskID)
	val.Set("show_raw", "1")
	val.Set("wide_logs", "true")
	return &url.URL{
		Scheme:   "https",
		Host:     swarmingHostname,
		Path:     "task",
		RawQuery: val.Encode(),
	}
}

// botPageURL returns a URL to a human-consumable page of a swarming bot.
// Supports host aliases.
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

func (b swarmingURLBuilder) BuildLink(l *annopb.AnnotationLink) *ui.Link {
	switch t := l.Value.(type) {
	case *annopb.AnnotationLink_LogdogStream:
		ls := t.LogdogStream

		link := ui.NewLink(l.Label, fmt.Sprintf("%s/%s/%s", URLBase, b, ls.Name), "")
		if link.Label == "" {
			link.Label = ls.Name
		}
		link.AriaLabel = fmt.Sprintf("log link for %s", link.Label)
		return link

	case *annopb.AnnotationLink_Url:
		return ui.NewLink(l.Label, t.Url, fmt.Sprintf("step link for %s", l.Label))

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

// getSwarmingHost returns default hostname if host is empty.
// If host is not empty and not allowed, returns an error.
func getSwarmingHost(c context.Context, host string) (string, error) {
	settings := config.GetSettings(c)
	if settings.Swarming == nil {
		err := errors.New("swarming not in settings")
		logging.WithError(err).Errorf(c, "Go configure swarming in the settings page.")
		return "", err
	}

	if host == "" || host == settings.Swarming.DefaultHost {
		return settings.Swarming.DefaultHost, nil
	}
	// If it is specified, validate the hostname.
	for _, allowed := range settings.Swarming.AllowedHosts {
		if host == allowed {
			return host, nil
		}
	}
	return "", grpcutil.InvalidArgumentTag.Apply(

		// GetBuild returns a milo build from a swarming task id.
		errors.New("unknown swarming host"))
}

func GetBuild(c context.Context, host, taskID string) (*ui.MiloBuildLegacy, error) {
	if taskID == "" {
		return nil, grpcutil.InvalidArgumentTag.Apply(errors.New("no swarming task id"))
	}

	sf, err := newProdService(c, host)
	if err != nil {
		return nil, err
	}

	return SwarmingBuildImpl(c, sf, taskID)
}

// RedirectsFromTask returns either
//   - The ID of the buildbucket build corresponding to this task. OR
//   - The build.proto logdog stream from this swarming task.
//
// If the task does not represent a buildbucket build, returns (0, "", nil).
func RedirectsFromTask(c context.Context, host, taskID string) (int64, string, error) {
	sf, err := newProdService(c, host)
	if err != nil {
		return 0, "", err
	}

	res, err := sf.GetSwarmingRequest(c, taskID)
	if err != nil {
		return 0, "", err
	}

	for _, t := range res.Tags {
		const bbPrefix = "buildbucket_build_id:"
		if strings.HasPrefix(t, bbPrefix) {
			value := t[len(bbPrefix):]
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				logging.Errorf(c, "failed to parse buildbucket_build_id tag %q as int64: %s", value, err)
				return 0, "", nil
			}
			return id, "", nil
		}

		const ldPrefix = "log_location:"
		if strings.HasPrefix(t, ldPrefix) {
			url := t[len(ldPrefix):]
			url = strings.TrimPrefix(url, "logdog://")
			return 0, url, nil
		}
	}
	return 0, "", nil
}
