// Copyright 2020 The LUCI Authors.
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

package rpc

import (
	"context"
	"crypto/subtle"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

type tagValidationMode int

const (
	TagNew tagValidationMode = iota
	TagAppend
)

const (
	buildSetMaxLength = 1024
	// summaryMardkdownMaxLength is the maximum size of Build.summary_markdown field in bytes.
	// Find more details at https://godoc.org/go.chromium.org/luci/buildbucket/proto#Build
	summaryMarkdownMaxLength = 4 * 1000

	// buildOutputPropertiesMaxBytes is the maximum length of build.output.properties.
	// If bytes exceeds this maximum, Buildbucket will reject this request.
	buildOutputPropertiesMaxBytes = 1000 * 1000
)

const UserPackageDir = "cipd_bin_packages"

// BbagentUtilPkgDir is the directory containing packages that bbagent uses.
const BbagentUtilPkgDir = "bbagent_utility_packages"

var (
	sha1Regex          = regexp.MustCompile(`^[a-f0-9]{40}$`)
	reservedKeys       = stringset.NewFromSlice("build_address")
	gitilesCommitRegex = regexp.MustCompile(`^commit/gitiles/([^/]+)/(.+?)/\+/([a-f0-9]{40})$`)
	gerritCLRegex      = regexp.MustCompile(`^patch/gerrit/([^/]+)/(\d+)/(\d+)$`)
)

func init() {
	bqlog.RegisterSink(bqlog.Sink{
		Prototype: &pb.PRPCRequestLog{},
		Table:     "prpc_request_log",
	})
}

// bundler is the key to a *bqlog.Bundler in the context.
var bundlerKey = "bundler"

// withBundler returns a new context with the given *bqlog.Bundler set.
func withBundler(ctx context.Context, b *bqlog.Bundler) context.Context {
	return context.WithValue(ctx, &bundlerKey, b)
}

// getBundler returns the *bqlog.Bundler installed in the current context.
// Panics if there isn't one.
func getBundler(ctx context.Context) *bqlog.Bundler {
	return ctx.Value(&bundlerKey).(*bqlog.Bundler)
}

// logToBQ logs a PRPC request log for this request to BigQuery (best-effort).
func logToBQ(ctx context.Context, id, parent, methodName string) {
	user := auth.CurrentIdentity(ctx)
	if user.Kind() == identity.User && !strings.HasSuffix(string(user), ".gserviceaccount.com") {
		user = ""
	}
	cTime := getStartTime(ctx)
	duration := int64(0)
	if !cTime.IsZero() {
		duration = clock.Now(ctx).Sub(cTime).Microseconds()
	}
	getBundler(ctx).Log(ctx, &pb.PRPCRequestLog{
		Id:           id,
		Parent:       parent,
		CreationTime: cTime.UnixNano() / 1000,
		Duration:     duration,
		Method:       methodName,
		User:         string(user),
	})
}

// commonPostlude converts an appstatus error to a gRPC error and logs it.
// Requires a *bqlog.Bundler in the context (see commonPrelude).
func commonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	logToBQ(ctx, trace.SpanContext(ctx), "", methodName)
	return appstatus.GRPCifyAndLog(ctx, err)
}

// teeErr saves `err` in `keep` and then returns `err`
func teeErr(err error, keep *error) error {
	*keep = err
	return err
}

// timeKey is the key to a time.Time in the context.
var timeKey = "start time"

// withStartTime returns a new context with the given time.Time set.
func withStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, &timeKey, t)
}

// getStartTime returns the time.Time installed in the current context.
func getStartTime(ctx context.Context) time.Time {
	if t, ok := ctx.Value(&timeKey).(time.Time); ok {
		return t
	}
	return time.Time{}
}

// commonPrelude logs debug information about the request and installs a
// start time and *bqlog.Bundler in the current context.
func commonPrelude(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	logging.Debugf(ctx, "%q called %q with request %s", auth.CurrentIdentity(ctx), methodName, proto.MarshalTextString(req))
	return withBundler(withStartTime(ctx, clock.Now(ctx)), &bqlog.Default), nil
}

func validatePageSize(pageSize int32) error {
	if pageSize < 0 {
		return errors.Reason("page_size cannot be negative").Err()
	}
	return nil
}

// getBuild returns the build with the given ID or NotFound appstatus if it is
// not found.
func getBuild(ctx context.Context, id int64) (*model.Build, error) {
	bld := &model.Build{ID: id}
	switch err := datastore.Get(ctx, bld); {
	case err == datastore.ErrNoSuchEntity:
		return nil, perm.NotFoundErr(ctx)
	case err != nil:
		return nil, errors.Annotate(err, "error fetching build with ID %d", id).Err()
	default:
		return bld, nil
	}
}

// validateTags validates build tags.
//
// tagValidationMode should be one of the enum - TagNew, TagAppend
// Note: Duplicate tags can pass the validation, which will be eventually deduplicated when storing into DB.
func validateTags(tags []*pb.StringPair, m tagValidationMode) error {
	if tags == nil {
		return nil
	}
	var k, v string
	var seenBuilderTagValue string
	for _, tag := range tags {
		k = tag.Key
		v = tag.Value
		if strings.Contains(k, ":") {
			return errors.Reason(`tag key "%s" cannot have a colon`, k).Err()
		}
		if m == TagAppend && buildbucket.DisallowedAppendTagKeys.Has(k) {
			return errors.Reason(`tag key "%s" cannot be added to an existing build`, k).Err()
		}
		if k == "buildset" {
			if err := validateBuildSet(v); err != nil {
				return err
			}
		}
		if k == "builder" {
			if seenBuilderTagValue == "" {
				seenBuilderTagValue = v
			} else if v != seenBuilderTagValue {
				return errors.Reason(`tag "builder:%s" conflicts with tag "builder:%s"`, v, seenBuilderTagValue).Err()
			}
		}
		if reservedKeys.Has(k) {
			return errors.Reason(`tag "%s" is reserved`, k).Err()
		}
	}
	return nil
}

func validateBuildSet(bs string) error {
	if len("buildset:")+len(bs) > buildSetMaxLength {
		return errors.Reason("buildset tag is too long").Err()
	}

	// Verify that a buildset with a known prefix is well formed.
	if strings.HasPrefix(bs, "commit/gitiles/") {
		matches := gitilesCommitRegex.FindStringSubmatch(bs)
		if len(matches) == 0 {
			return errors.Reason(`does not match regex "%s"`, gitilesCommitRegex).Err()
		}
		project := matches[2]
		if strings.HasPrefix(project, "a/") {
			return errors.Reason(`gitiles project must not start with "a/"`).Err()
		}
		if strings.HasSuffix(project, ".git") {
			return errors.Reason(`gitiles project must not end with ".git"`).Err()
		}
	} else if strings.HasPrefix(bs, "patch/gerrit/") {
		if !gerritCLRegex.MatchString(bs) {
			return errors.Reason(`does not match regex "%s"`, gerritCLRegex).Err()
		}
	}
	return nil
}

func validateSummaryMarkdown(md string) error {
	if len(md) > summaryMarkdownMaxLength {
		return errors.Reason("too big to accept (%d > %d bytes)", len(md), summaryMarkdownMaxLength).Err()
	}
	return nil
}

// TODO(ddoman): move proto validator functions to protoutil.

// validateCommitWithRef checks if `cm` is a valid commit with a ref.
func validateCommitWithRef(cm *pb.GitilesCommit) error {
	if cm.GetRef() == "" {
		return errors.Reason(`ref is required`).Err()
	}
	return validateCommit(cm)
}

// validateCommit validates the given Gitiles commit.
func validateCommit(cm *pb.GitilesCommit) error {
	if cm.GetHost() == "" {
		return errors.Reason("host is required").Err()
	}
	if cm.GetProject() == "" {
		return errors.Reason("project is required").Err()
	}

	if cm.GetRef() != "" {
		if !strings.HasPrefix(cm.Ref, "refs/") {
			return errors.Reason("ref must match refs/.*").Err()
		}
	} else if cm.Position != 0 {
		return errors.Reason("position requires ref").Err()
	}

	if cm.GetId() != "" && !sha1Regex.MatchString(cm.Id) {
		return errors.Reason("id must match %q", sha1Regex).Err()
	}
	if cm.GetRef() == "" && cm.GetId() == "" {
		return errors.Reason("one of id or ref is required").Err()
	}
	return nil
}

// Returned from validateToken if no token is found; Some uses of validateToken
// allow a missing token (such as establishing parent->child relationship during
// ScheduleBuild).
var errMissingToken = errors.New("token is missing", grpcutil.UnauthenticatedTag)

// Maps token purpose to the series of headers to check.
var tokenHeaders = map[pb.TokenBody_Purpose][]string{
	pb.TokenBody_TASK: {buildbucket.BuildbucketTokenHeader},
	pb.TokenBody_BUILD: {
		buildbucket.BuildbucketTokenHeader,
		// TODO(crbug.com/1031205): remove buildbucket.BuildTokenHeader.
		buildbucket.BuildTokenHeader,
	},
}

// validateToken validates the update token from the header.
//
// `bID` is used to retrieve the build with the old build token for validation,
// if known (i.e. in UpdateBuild, `bID` can be retrieved from the request). If
// it is 0, we are only rely on the build token contents to be self-validating
// (i.e. when scheduling a child build).
//
// If no token is found, this returns `errMissingToken`, which is tagged as
// Unauthenticated.
//
// If the token is encrypted, this will early exit if the token encryption is
// wrong, a non-zero bID doesn't match the token body, or if the token's purpose
// doesn't match the expected `purpose`. Once all tokens are ecnrypted,
// validateToken should move to the top of handler functions whcih require
// a token.
//
// Finally, this compares the raw token in the header against the token recorded
// in the Build entity, to ensure that Buildbucket didn't hand out two (or more)
// tokens for this Build.
// BUG(crbug.com/1401174) This raw token comparison only applies to
// BUILD-purposed tokens.
func validateToken(ctx context.Context, bID int64, purpose pb.TokenBody_Purpose) (*pb.TokenBody, *model.Build, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	var buildTok string
	for _, header := range tokenHeaders[purpose] {
		buildToks := md.Get(header)
		if len(buildToks) == 1 {
			buildTok = buildToks[0]
			if header == buildbucket.BuildTokenHeader {
				logging.Infof(ctx, "BUG(crbug.com/1031205): got build-token for %d", bID)
			}
			break
		} else if len(buildToks) > 1 {
			return nil, nil, errors.Reason("multiple build tokens are provided").Err()
		}
	}
	if buildTok == "" {
		return nil, nil, errMissingToken
	}

	tokBody, err := buildtoken.ParseToTokenBody(ctx, buildTok, bID, purpose)
	if err != nil {
		// There's something wrong with the build token itself.
		//
		// Note that if the token was encrypted, we bail out here.
		// ParseToTokenBody has also checked the build id and purpose of the token, if
		// it did successfully decrypt.
		return nil, nil, err
	}

	if tokBody != nil && bID == 0 {
		bID = tokBody.BuildId
	}

	if bID == 0 {
		return nil, nil, errors.Reason("failed to get build id to validate the build token").Err()
	}

	bld, err := getBuild(ctx, bID)
	if err != nil {
		return nil, nil, err
	}

	// Validate it against the token stored in the build entity.
	//
	// This can catch cases where buildbucket issued two tasks, both with valid
	// tokens for this build, but only one of them will be saved in datastore.
	//
	// BUG(crbug.com/1401174) - We only do this check for UpdateBuild calls
	// currently. Really this whole protocol needs to be reworked.
	if purpose == pb.TokenBody_BUILD {
		if subtle.ConstantTimeCompare([]byte(buildTok), []byte(bld.UpdateToken)) == 1 {
			return tokBody, bld, nil
		}
		return nil, nil, perm.NotFoundErr(ctx)
	}

	return tokBody, bld, nil
}
