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

package notify

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/pubsub"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
)

var buildbucketPubSub = metric.NewCounter(
	"luci/notify/buildbucket-pubsub",
	"Number of received Buildbucket PubSub messages",
	nil,
	// "success", "transient-failure" or "permanent-failure"
	field.String("status"),
)

func getBuilderID(b *buildbucketpb.Build) string {
	return fmt.Sprintf("%s/%s", b.Builder.Bucket, b.Builder.Builder)
}

// EmailNotify contains information for delivery and personalization of notification emails.
type EmailNotify struct {
	Email         string `json:"email"`
	Template      string `json:"template"`
	MatchingSteps []*buildbucketpb.Step
}

// sortEmailNotify sorts a list of EmailNotify by Email, then Template.
func sortEmailNotify(en []EmailNotify) {
	sort.Slice(en, func(i, j int) bool {
		first := en[i]
		second := en[j]
		emailResult := strings.Compare(first.Email, second.Email)
		if emailResult == 0 {
			return strings.Compare(first.Template, second.Template) < 0
		}
		return emailResult < 0
	})
}

// extractEmailNotifyValues extracts EmailNotify slice from the build.
// TODO(nodir): remove parametersJSON once clients move to properties.
func extractEmailNotifyValues(build *buildbucketpb.Build, parametersJSON string) ([]EmailNotify, error) {
	const propertyName = "email_notify"
	value := build.GetOutput().GetProperties().GetFields()[propertyName]
	if value == nil {
		value = build.GetInput().GetProperties().GetFields()[propertyName]
	}
	if value != nil {
		notifiesPB := value.GetListValue().GetValues()
		ret := make([]EmailNotify, len(notifiesPB))
		for i, notifyPB := range notifiesPB {
			notifyFields := notifyPB.GetStructValue().GetFields()
			ret[i] = EmailNotify{
				Email:    notifyFields["email"].GetStringValue(),
				Template: notifyFields["template"].GetStringValue(),
				// MatchingSteps is left blank, as it is only available for recipients
				// derived from Notifications with step filters.
			}
		}
		return ret, nil
	}

	if parametersJSON == "" {
		return nil, nil
	}
	// json equivalent: {"email_notify": [{"email": "<address>"}, ...]}
	var output struct {
		EmailNotify []EmailNotify `json:"email_notify"`
	}

	if err := json.NewDecoder(strings.NewReader(parametersJSON)).Decode(&output); err != nil {
		return nil, errors.Annotate(err, "invalid msg.ParametersJson").Err()
	}
	return output.EmailNotify, nil
}

func randInRange(upper, lower float64) float64 {
	return lower + rand.Float64()*(upper-lower)
}

func putWithRetry(c context.Context, entity any) error {
	// Retry with exponential backoff. The limit we care about here is the limit
	// on writes to entities within an entity group, which is once per second.
	//
	// By waiting for 1 second we'll guarantee not to conflict with the write
	// that caused contention the first time we failed. We double the delay each
	// time, and randomly shift over the interval to avoid clustering.

	delay := 1.0
	var err error

	// Retry up to 4 times. Past that point the delay is becoming too long to be
	// reasonable.
	for i := 0; i < 4; i++ {
		err = datastore.Put(c, entity)
		if err == nil {
			return nil
		} else if status.Code(err) != codes.Aborted {
			// Datastore documentation says that Aborted is returned in the case
			// of contention, which is the only case we want to retry on.
			// TODO: Perhaps also retry on some others like Unavailable or
			// DeadlineExceeded?
			return err
		}

		clock.Sleep(c, time.Duration(float64(time.Second)*randInRange(delay, delay*2)))
		delay *= 2
	}

	return err
}

// handleBuild processes a Build and sends appropriate notifications.
//
// This function should serve as documentation of the process of going from
// a Build to sent notifications. It also should explicitly handle ACLs and
// stop the process of handling notifications early to avoid wasting compute
// time.
//
// getCheckout produces the associated source checkout for a build, if available.
// It's passed in as a parameter in order to mock it for testing.
//
// history is a function that contacts gitiles to obtain the git history for
// revision ordering purposes. It's passed in as a parameter in order to mock it
// for testing.
func handleBuild(c context.Context, build *Build, getCheckout CheckoutFunc, history HistoryFunc) error {
	luciProject := build.Builder.Project
	project := &config.Project{Name: luciProject}
	switch ex, err := datastore.Exists(c, project); {
	case err != nil:
		return err
	case !ex.All():
		logging.Infof(c, "Build project not tracked by LUCI Notify, ignoring...")
		return nil // This project is not tracked by luci-notify
	}

	// checkout is only used to compute the blamelist
	// As blamelist is not a "critical" feature of luci-notify, if there is an
	// error getting the checkout (mostly because there is no source manifest)
	// we should not throw 500, but just log the error, and inform the builder
	// owner that they are missing source manifest in their builds
	logdogContext, _ := context.WithTimeout(c, LOGDOG_REQUEST_TIMEOUT)
	checkout, err := getCheckout(logdogContext, build)
	if err != nil {
		// TODO (crbug.com/1058190): log the error and let the owner know
		logging.Warningf(c, "Got error when getting source manifest for build %v", err)
	}

	// Get the Builder for the first time, and initialize if there's nothing there.
	builderID := getBuilderID(&build.Build)
	templateInput := &notifypb.TemplateInput{
		BuildbucketHostname: build.BuildbucketHostname,
		Build:               &build.Build,
	}

	// Set up the initial list of recipients, derived from the build.
	recipients := make([]EmailNotify, len(build.EmailNotify))
	copy(recipients, build.EmailNotify)

	// Helper functions for notifying and updating tree closer status.
	notifyNoBlame := func(c context.Context, b config.Builder, oldStatus buildbucketpb.Status) error {
		notifications := Filter(c, &b.Notifications, oldStatus, &build.Build)
		recipients = append(recipients, ComputeRecipients(c, notifications, nil, nil)...)
		templateInput.OldStatus = oldStatus
		return Notify(c, recipients, templateInput)
	}
	notifyAndUpdateTrees := func(c context.Context, b config.Builder, oldStatus buildbucketpb.Status) error {
		return parallel.FanOutIn(func(ch chan<- func() error) {
			ch <- func() error { return notifyNoBlame(c, b, oldStatus) }
			ch <- func() error { return UpdateTreeClosers(c, build, oldStatus) }
		})
	}

	gCommit := build.Input.GetGitilesCommit()
	buildCreateTime := build.CreateTime.AsTime()

	var keepGoing bool
	var builder config.Builder
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		// Reset these values everytime the transaction is retried, to avoid
		// leaking state from a previous attempt to this attempt.
		keepGoing = false
		builder = config.Builder{
			ProjectKey: datastore.KeyForObj(c, project),
			ID:         builderID,
		}

		switch err := datastore.Get(c, &builder); {
		case err == datastore.ErrNoSuchEntity:
			// Even if the builder isn't found, we may still want to notify if the build
			// specifies email addresses to notify.
			logging.Infof(c, "No builder %q found for project %q", builderID, luciProject)
			return Notify(c, recipients, templateInput)
		case err != nil:
			return errors.Annotate(err, "failed to get builder").Tag(transient.Tag).Err()
		}

		// Create a new builder as a copy of the old, updated with build information.
		updatedBuilder := builder
		updatedBuilder.Status = build.Status
		updatedBuilder.BuildTime = buildCreateTime
		if len(checkout) > 0 {
			updatedBuilder.GitilesCommits = checkout.ToGitilesCommits()
		}

		if builder.Repository == "" {
			// No repository specified, so we follow build time ordering.
			if builder.BuildTime.Before(buildCreateTime) {
				// The build is in-order with respect to build time, so notify normally.
				if err := notifyAndUpdateTrees(c, builder, builder.Status); err != nil {
					return err
				}
				return putWithRetry(c, &updatedBuilder)
			}
			logging.Infof(c, "Found build with old time")

			// Don't update trees, since it's out of order.
			return notifyNoBlame(c, builder, 0)
		}
		// Repository specified, so we follow commit ordering.

		if gCommit == nil || (gCommit != nil && gCommit.Id == "") {
			// If there's no revision information, the build must be treated as out-
			// of-order. For such builds we only perform non-on_change notifications,
			// and do not update trees.
			logging.Infof(c, "No revision information found for this build, treating as out-of-order...")
			return notifyNoBlame(c, builder, 0)
		}

		builderRepoHost, builderRepoProject, _ := gitiles.ParseRepoURL(builder.Repository)
		if builderRepoHost != gCommit.Host || builderRepoProject != gCommit.Project {
			logging.Infof(c, "Builder %s triggered by commit to https://%s/%s"+
				"instead of known https://%s, treating as out-of-order...",
				builderID, gCommit.Host, gCommit.Project, builder.Repository)
			// Only perform non-on_change notifications.
			return notifyNoBlame(c, builder, 0)
		}

		// Update the new builder with revision information as we know it's now available.
		updatedBuilder.Revision = gCommit.Id

		// If there's no revision information on the Builder, this means the Builder
		// is uninitialized. Notify about the build as best as we can and then store
		// the updated builder.
		if builder.Revision == "" {
			if err := notifyAndUpdateTrees(c, builder, 0); err != nil {
				return err
			}
			return putWithRetry(c, &updatedBuilder)
		}
		keepGoing = true
		return nil
	}, nil)
	if err != nil {
		return err
	}
	if !keepGoing {
		return nil
	}

	// We have a builder with a Repository set (i.e. following commit ordering),
	// and a build with commit ID set. Continue processing notifications.

	// Get the revision history for the build-related commit.
	commits, err := history(c, luciProject, gCommit.Host, gCommit.Project, builder.Revision, gCommit.Id)
	if err != nil {
		if status.Code(err) == codes.DeadlineExceeded {
			// Retrieve git history might time out (b/327294463), don't perform notification and log a warning.
			logging.Warningf(c, "retrieve git history timeout, maybe related to b/327294463 %v", err)
			return nil
		}
		return errors.Annotate(err, "failed to retrieve git history for input commit").Err()
	}
	if len(commits) == 0 {
		logging.Debugf(c, "Found build with old commit, not updating tree closers")
		return notifyNoBlame(c, builder, 0)
	}

	// Get the blamelist logs, if needed.
	var aggregateLogs Logs
	aggregateRepoAllowset := BlamelistRepoAllowset(builder.Notifications)
	if len(aggregateRepoAllowset) > 0 && len(checkout) > 0 {
		oldCheckout := NewCheckout(builder.GitilesCommits)
		aggregateLogs, err = ComputeLogs(c, luciProject, oldCheckout, checkout.Filter(aggregateRepoAllowset), history)
		if err != nil {
			return errors.Annotate(err, "failed to compute logs").Err()
		}
	}

	// Update `builder`, and check if we need to store a newer version, then store it.
	oldRepository := builder.Repository
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, &builder); {
		case err == datastore.ErrNoSuchEntity:
			return errors.New("builder deleted between datastore.Get calls")
		case err != nil:
			return err
		}

		// If the builder's repository got updated in the meanwhile, we need to throw a
		// transient error and retry this whole thing.
		if builder.Repository != oldRepository {
			return errors.Reason("failed to notify because builder repository updated").Tag(transient.Tag).Err()
		}

		// Create a new builder as a copy of the old, updated with build information.
		updatedBuilder := builder
		updatedBuilder.Status = build.Status
		updatedBuilder.BuildTime = buildCreateTime
		updatedBuilder.Revision = gCommit.Id
		if len(checkout) > 0 {
			updatedBuilder.GitilesCommits = checkout.ToGitilesCommits()
		}

		index := commitIndex(commits, builder.Revision)
		outOfOrder := false
		switch {
		// If the revision is not found, we can conclude that the Builder has
		// advanced beyond gCommit.Revision. This is because:
		//   1) builder.Revision only ever moves forward.
		//   2) commits contains the git history up to gCommit.Revision.
		case index < 0:
			logging.Debugf(c, "Found build with old commit during transaction.")
			outOfOrder = true

		// If the revision is current, check build creation time.
		case index == 0 && builder.BuildTime.After(buildCreateTime):
			logging.Debugf(c, "Found build with the same commit but an old time.")
			outOfOrder = true
		}

		if outOfOrder {
			// If the build is out-of-order, we want to ignore only on_change notifications,
			// and not update trees.
			return notifyNoBlame(c, builder, 0)
		}

		// Notify, and include the blamelist.
		n := Filter(c, &builder.Notifications, builder.Status, &build.Build)
		recipients = append(recipients, ComputeRecipients(c, n, commits[:index], aggregateLogs)...)
		templateInput.OldStatus = builder.Status

		return parallel.FanOutIn(func(ch chan<- func() error) {
			ch <- func() error { return Notify(c, recipients, templateInput) }
			ch <- func() error { return putWithRetry(c, &updatedBuilder) }
			ch <- func() error { return UpdateTreeClosers(c, build, 0) }
		})
	}, nil)
	return errors.Annotate(err, "failed to save builder").Tag(transient.Tag).Err()
}

func newBuildsClient(c context.Context, host, project string) (buildbucketpb.BuildsClient, error) {
	t, err := auth.GetRPCTransport(c, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}
	return buildbucketpb.NewBuildsPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: t},
		Host: host,
	}), nil
}

// BuildbucketPubSubHandler is the main entrypoint for a new update from buildbucket's pubsub.
//
// This handler delegates the actual processing of the build to handleBuild.
func BuildbucketPubSubHandler(ctx context.Context, message pubsub.Message, buildMsg *buildbucketpb.BuildsV2PubSub) (err error) {
	defer func() {
		buildbucketPubSub.Add(ctx, 1, errStatus(err))
	}()

	build, err := extractBuild(ctx, buildMsg)
	if err != nil {
		return errors.Annotate(err, "failed to extract build").Err()
	}
	return handleBuild(ctx, build, srcmanCheckout, gitilesHistory)
}

func errStatus(err error) string {
	if err == nil {
		return "success"
	}
	if transient.Tag.In(err) {
		return "transient-failure"
	} else {
		return "permanent-failure"
	}
}

// Build is buildbucketpb.Build along with the parsed 'email_notify' values.
type Build struct {
	BuildbucketHostname string
	buildbucketpb.Build
	EmailNotify []EmailNotify
}

func extractBuild(c context.Context, buildsV2Msg *buildbucketpb.BuildsV2PubSub) (*Build, error) {
	// Double check the received build is completed, although the new subscription has a filter for it.
	if buildsV2Msg.Build.Status&buildbucketpb.Status_ENDED_MASK != buildbucketpb.Status_ENDED_MASK {
		logging.Infof(c, "Received build %d that hasn't completed yet, ignoring...", buildsV2Msg.Build.GetId())
		return nil, nil
	}
	largeFieldsData, err := zlibDecompress(buildsV2Msg.BuildLargeFields)
	if err != nil {
		return nil, errors.Annotate(err, "failed to decompress build_large_fields for build %d", buildsV2Msg.Build.GetId()).Err()
	}
	largeFields := &buildbucketpb.Build{}
	if err := proto.Unmarshal(largeFieldsData, largeFields); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal build_large_fields for build %d", buildsV2Msg.Build.GetId()).Err()
	}
	proto.Merge(buildsV2Msg.Build, largeFields)

	emails, err := extractEmailNotifyValues(buildsV2Msg.Build, "")
	if err != nil {
		return nil, errors.Annotate(err, "could not decode email_notify in builds_v2 pubsub message for build %d", buildsV2Msg.Build.GetId()).Err()
	}

	return &Build{
		BuildbucketHostname: buildsV2Msg.Build.GetInfra().GetBuildbucket().GetHostname(),
		Build:               *buildsV2Msg.Build,
		EmailNotify:         emails,
	}, nil
}

// zlibDecompress decompresses data using zlib.
func zlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	originalData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	return originalData, nil
}
