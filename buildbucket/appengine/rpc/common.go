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
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"

	"go.chromium.org/luci/buildbucket"
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

	// Sanity length limitation for build tokens to allow us to quickly reject
	// potentially abusive inputs.
	buildTokenMaxLength = 200
)

const UserPackageDir = "cipd_bin_packages"

var (
	sha1Regex          = regexp.MustCompile(`^[a-f0-9]{40}$`)
	reservedKeys       = stringset.NewFromSlice("build_address")
	gitilesCommitRegex = regexp.MustCompile(`^commit/gitiles/([^/]+)/(.+?)/\+/([a-f0-9]{40})$`)
	gerritCLRegex      = regexp.MustCompile(`^patch/gerrit/([^/]+)/(\d+)/(\d+)$`)
)

var buildTokenInOldFormat = errors.BoolTag{Key: errors.NewTagKey("build token in the old format")}

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
// tagValidationMode should be one of the enum - TagNew, TagAppend
// Note: Duplicate tags can pass the validation, which will be eventually deduplicated when storing into DB.
func validateTags(tags []*pb.StringPair, m tagValidationMode) error {
	if tags == nil {
		return nil
	}
	var k, v string
	var seenBuilderTagValue string
	var seenGitilesCommit string
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
			if gitilesCommitRegex.MatchString(v) {
				if seenGitilesCommit != "" && v != seenGitilesCommit {
					return errors.Reason(`tag "buildset:%s" conflicts with tag "buildset:%s"`, v, seenGitilesCommit).Err()
				}
				seenGitilesCommit = v
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

// tokenBody deserialize the build token and returns the token body.
func tokenBody(bldTok string) (*pb.TokenBody, error) {
	if len(bldTok) > buildTokenMaxLength {
		return nil, errors.Reason("build token %s is too long", bldTok).Err()
	}
	tokBytes, err := base64.RawURLEncoding.DecodeString(bldTok)
	if err != nil {
		return nil, errors.Reason("error decoding token").Err()
	}

	msg := &pb.TokenEnvelope{}
	if err := proto.Unmarshal(tokBytes, msg); err != nil {
		return nil, errors.Reason("error unmarshalling token").Tag(buildTokenInOldFormat).Err()
	}

	if msg.Version != pb.TokenEnvelope_UNENCRYPTED_PASSWORD_LIKE {
		return nil, errors.Reason("token with version %d is not supported", msg.Version).Err()
	}

	body := &pb.TokenBody{}
	if err := proto.Unmarshal(msg.Payload, body); err != nil {
		return nil, errors.Reason("error unmarshalling token payload").Err()
	}
	return body, nil
}

// validateBuildToken validates the update token from the header.
//
// bID is mainly used to retrieve the build with the old build token
// for validation, if known (i.e. in UpdateBuild, bID can be retrieved from
// the request).
// It can also be 0, witch means we only rely on the build token itself to be
// self-validating (i.e. when schedule a child build).
//
// requireToken is a flag to indicate if the build token is required. For example
// build token is required in UpdateBuild, but in ScheduleBuild, build token is
// only attached in the context when scheduling a child build.
// So missing the build token in ScheduleBuild doesn't necessarily mean there's an
// error.
func validateBuildToken(ctx context.Context, bID int64, requireToken bool) (*pb.TokenBody, *model.Build, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	buildToks := md.Get(buildbucket.BuildbucketTokenHeader)
	if len(buildToks) == 0 {
		// TODO(crbug.com/1031205): remove buildbucket.BuildTokenHeader.
		buildToks = md.Get(buildbucket.BuildTokenHeader)
		if len(buildToks) > 0 {
			logging.Infof(ctx, "got build-token for %d", bID)
		}
	}
	if len(buildToks) == 0 {
		if !requireToken {
			return nil, nil, nil
		}
		return nil, nil, appstatus.Errorf(codes.Unauthenticated, "missing header %q", buildbucket.BuildbucketTokenHeader)
	}

	if len(buildToks) > 1 {
		return nil, nil, errors.Reason("multiple build tokens are provided").Err()
	}

	bldTok := buildToks[0]
	tokBody, err := tokenBody(bldTok)
	// There's something wrong with the build token itself.
	if err != nil && !buildTokenInOldFormat.In(err) {
		return nil, nil, err
	}

	if tokBody != nil {
		if bID == 0 {
			bID = tokBody.BuildId
		} else if tokBody.BuildId != bID {
			return nil, nil, errors.Reason("unmatched requested build id and build token").Err()
		}
	}

	if bID == 0 {
		return nil, nil, errors.Reason("failed to get build id to validate the build token").Err()
	}

	bld, err := getBuild(ctx, bID)
	if err != nil {
		return nil, nil, err
	}

	// validate it against the token stored in the build entity.
	if subtle.ConstantTimeCompare([]byte(bldTok), []byte(bld.UpdateToken)) == 1 {
		return tokBody, bld, nil
	}
	return nil, nil, perm.NotFoundErr(ctx)
}
