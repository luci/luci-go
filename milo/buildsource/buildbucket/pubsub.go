package buildbucket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	bucketApi "github.com/luci/luci-go/common/api/buildbucket/buildbucket/v1"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/router"
)

var (
	buildCounter = metric.NewCounter(
		"luci/milo/buildbucket_pubsub/builds",
		"The number of buildbucket builds received by Milo from PubSub",
		nil,
		field.String("bucket"),
		// True for luci build, False for non-luci (ie buildbot) build.
		field.Bool("luci"),
		// Status can be "COMPLETED", "SCHEDULED", or "STARTED"
		field.String("status"),
		// Action can be one of 3 options.  "New", "Replaced", "Rejected".
		field.String("action"))
)

var (
	errNoLogLocation = errors.New("log_location tag not found")
	errNoProject     = errors.New("project tag not found")
)

type parameters struct {
	builderName string `json:"builder_name"`
	properties  string `json:"properties"`
}

func isLUCI(build *bucketApi.ApiCommonBuildMessage) bool {
	// All luci buckets are assumed to be prefixed with luci.
	return strings.HasPrefix(build.Bucket, "luci.")
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	err := pubSubHandlerImpl(ctx.Context, ctx.Request)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "error while updating buildbucket")
	}
	if transient.Tag.In(err) {
		// Transient errors are 500 so that PubSub retries them.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	} else {
		// No errors or non-transient errors are 200s so that PubSub does not retry
		// them.
		ctx.Writer.WriteHeader(http.StatusOK)
	}

}

func handlePubSubBuild(c context.Context, build *bucketApi.ApiCommonBuildMessage) error {
	if err := buildCounter.Add(
		c, 1, build.Bucket, isLUCI(build), build.Status, "New"); err != nil {
		logging.WithError(err).Warningf(c, "Failed to send metric")
	}
	logging.Debugf(c, "Received build %#v", build)
	// TODO(hinoka): Save this into datastore.
	return nil
}

// This returns 500 (Internal Server Error) if it encounters a transient error,
// and returns 200 (OK) if everything is OK, or if it encounters a permanent error.
func pubSubHandlerImpl(c context.Context, r *http.Request) error {
	var data struct {
		Build    bucketApi.ApiCommonBuildMessage
		Hostname string
	}

	msg := common.PubSubSubscription{}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		logging.WithError(err).Errorf(c, "could not decode message:\n%s", r.Body)
		// This might be a transient error, e.g. when the json format changes
		// and Milo isn't updated yet.
		return transient.Tag.Apply(err)
	}
	bData, err := msg.GetData()
	if err != nil {
		logging.WithError(err).Errorf(c, "could not parse pubsub message string")
		return err
	}
	if err := json.Unmarshal(bData, &data); err != nil {
		logging.WithError(err).Errorf(c, "could not parse pubsub message data")
		return err
	}

	return handlePubSubBuild(c, &data.Build)
}
