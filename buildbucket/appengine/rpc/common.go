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
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type tagValidationMode int

const (
	TagNew tagValidationMode = iota
	TagAppend
)

const (
	buildSetMaxLength = 1024
)

const (
	// BbagentUtilPkgDir is the directory containing packages that bbagent uses.
	BbagentUtilPkgDir = "bbagent_utility_packages"
	// CipdClientDir is the directory containing cipd itself
	CipdClientDir  = "cipd"
	UserPackageDir = "cipd_bin_packages"
)

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
	logToBQ(ctx, trace.SpanContextFromContext(ctx).TraceID().String(), "", methodName)
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
	if len(md) > protoutil.SummaryMarkdownMaxLength {
		return errors.Reason("too big to accept (%d > %d bytes)", len(md), protoutil.SummaryMarkdownMaxLength).Err()
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
var errBadTokenAuth = errors.New("expected buildID and exactly one buildbucket token", grpcutil.UnauthenticatedTag)

// getBuildbucketToken extracts a singlar encoded build token from the current
// gRPC Metadata in `ctx`.
//
// Does not parse or validate the token in anyway (see validateToken for typical
// usage, or buildtoken.ParseToTokenBody for more specialized usage).
//
// `kitchenFallback` should only be supplied in cases where we need to fall back
// to kitchen's deprecated BuildTokenHeader.
//
// Returns errBadTokenAuth if the token is missing, or there is more than one.
func getBuildbucketToken(ctx context.Context, kitchenFallback bool) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	tokens := md.Get(buildbucket.BuildbucketTokenHeader)
	if len(tokens) == 0 && kitchenFallback {
		// TODO: Remove this when kitchen is removed.
		tokens = md.Get(buildbucket.BuildTokenHeader)
	}
	if len(tokens) == 1 {
		return tokens[0], nil
	}

	return "", errBadTokenAuth
}

// validateToken validates the build token from the header.
//
// The `purpose` and `bID` must match the purpose and build ID listed in the token.
//
// All errors that this would return are errBadTokenAuth.
// Details about token parsing are logged.
func validateToken(ctx context.Context, bID int64, purpose pb.TokenBody_Purpose) (*pb.TokenBody, error) {
	if bID <= 0 {
		return nil, errBadTokenAuth
	}
	buildTok, err := getBuildbucketToken(ctx, purpose == pb.TokenBody_BUILD)
	if err != nil {
		return nil, err
	}
	tok, err := buildtoken.ParseToTokenBody(ctx, buildTok, bID, purpose)
	if err != nil {
		return nil, err
	}
	return tok, nil
}
