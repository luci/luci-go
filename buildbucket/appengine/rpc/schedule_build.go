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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	cipdcommon "go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/caching"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// Allow hostnames permitted by
// https://www.rfc-editor.org/rfc/rfc1123#page-13. (Note that
// the 255 character limit must be seperately applied.)
var hostnameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]+(\.[a-z0-9-]+)*$`)

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

// validateExpirationDuration validates the given expiration duration.
func validateExpirationDuration(d *durationpb.Duration) error {
	switch {
	case d.GetNanos() != 0:
		return errors.Reason("nanos must not be specified").Err()
	case d.GetSeconds() < 0:
		return errors.Reason("seconds must not be negative").Err()
	case d.GetSeconds()%60 != 0:
		return errors.Reason("seconds must be a multiple of 60").Err()
	default:
		return nil
	}
}

// validateRequestedDimension validates the requested dimension.
func validateRequestedDimension(dim *pb.RequestedDimension) error {
	var err error
	switch {
	case teeErr(validateDimension(dim, true), &err) != nil:
		return err
	case dim.Key == "caches":
		return errors.New("key: caches may only be specified in builder configs (cr-buildbucket.cfg)")
	case dim.Key == "pool":
		return errors.New("key: pool may only be specified in builder configs (cr-buildbucket.cfg)")
	default:
		return nil
	}
}

// validateRequestedDimensions validates the requested dimensions.
func validateRequestedDimensions(dims []*pb.RequestedDimension) error {
	// A dim.key set which contains non-empty dim.value
	nonEmpty := stringset.New(len(dims))
	// A dim.key set which contains empty dim.value
	empty := stringset.New(len(dims))
	for i, dim := range dims {
		if err := validateRequestedDimension(dim); err != nil {
			return errors.Annotate(err, "[%d]", i).Err()
		}

		if dim.GetValue() == "" {
			if nonEmpty.Has(dim.Key) {
				return errors.Reason("contain both empty and non-empty value for the same key - %q", dim.Key).Err()
			}
			empty.Add(dim.Key)
		} else {
			if empty.Has(dim.Key) {
				return errors.Reason("contain both empty and non-empty value for the same key - %q", dim.Key).Err()
			}
			nonEmpty.Add(dim.Key)
		}
	}
	return nil
}

// validateExecutable validates the given executable.
func validateExecutable(exe *pb.Executable) error {
	var err error
	switch {
	case exe.GetCipdPackage() != "":
		return errors.Reason("cipd_package must not be specified").Err()
	case exe.GetCipdVersion() != "" && teeErr(cipdcommon.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Fmt("cipd_version: %w", err)
	default:
		return nil
	}
}

// validateGerritChange validates a given gerrit change.
func validateGerritChange(ch *pb.GerritChange) error {
	switch {
	case ch.GetChange() == 0:
		return errors.Reason("change must be specified").Err()
	case ch.Host == "":
		return errors.Reason("host must be specified").Err()
	case !hostnameRE.MatchString(ch.Host):
		return errors.Reason("host does not match pattern %q", hostnameRE).Err()
	case len(ch.Host) > 255:
		return errors.Reason("host must not exceed 255 characters").Err()
	case ch.Patchset == 0:
		return errors.Reason("patchset must be specified").Err()
	case ch.Project == "":
		return errors.Reason("project must be specified").Err()
	default:
		return nil
	}
}

// validateGerritChanges validates the given gerrit changes.
func validateGerritChanges(changes []*pb.GerritChange) error {
	for i, ch := range changes {
		if err := validateGerritChange(ch); err != nil {
			return errors.Annotate(err, "[%d]", i).Err()
		}
	}
	return nil
}

// validateNotificationConfig validates the given notification config.
func validateNotificationConfig(ctx context.Context, n *pb.NotificationConfig) error {
	switch {
	case n.GetPubsubTopic() == "":
		return errors.Reason("pubsub_topic must be specified").Err()
	case len(n.UserData) > 4096:
		return errors.Reason("user_data cannot exceed 4096 bytes").Err()
	}

	// Validate the topic exists and Buildbucket has the publishing permission.
	cloudProj, topicID, err := clients.ValidatePubSubTopicName(n.PubsubTopic)
	if err != nil {
		return errors.Annotate(err, "invalid pubsub_topic %s", n.PubsubTopic).Err()
	}

	// Check the global cache first to reduce calls to the actual IAM api.
	cache := caching.GlobalCache(ctx, "has_perm_on_pubsub_callback_topic")
	if cache == nil {
		logging.Warningf(ctx, "global has_perm_on_pubsub_callback_topic cache is not found")
	}
	switch hasPerm, err := cache.Get(ctx, n.PubsubTopic); {
	case err == caching.ErrCacheMiss:
	case err != nil:
		logging.Warningf(ctx, "failed to check %s from the global cache", n.PubsubTopic)
	case hasPerm != nil:
		return nil
	}

	// Check perm via the IAM api and save into the cache iff BB has the access on
	// that topic. Why not also caching the bad result? Because users will usually
	// correct the permission once they receive the bad response and retry again.
	// Caching the bad result means we have to figure out a way to invalidate the
	// cached item before it expires.
	client, err := clients.NewPubsubClient(ctx, cloudProj, "")
	if err != nil {
		return errors.Annotate(err, "failed to create a pubsub client").Err()
	}
	topic := client.Topic(topicID)
	switch perms, err := topic.IAM().TestPermissions(ctx, []string{"pubsub.topics.publish"}); {
	case err != nil:
		return errors.Annotate(err, "failed to check existence for topic %s", topic).Err()
	case len(perms) < 1:
		return errors.Reason("%s@appspot.gserviceaccount.com account doesn't have the 'pubsub.topics.publish' or 'pubsub.topics.get' permission for %s", info.AppID(ctx), topic).Err()
	default:
		if err := cache.Set(ctx, n.PubsubTopic, []byte{1}, 10*time.Hour); err != nil {
			logging.Warningf(ctx, "failed to save into has_perm_on_pubsub_callback_topic cache for %s", n.PubsubTopic)
		}
	}
	return nil
}

// prohibitedProperties is used to prohibit properties from being set (see
// validateProperties). Contains slices of path components forming a prohibited
// path. For example, to prohibit a property "a.b", add an element ["a", "b"].
var prohibitedProperties = [][]string{
	{"$recipe_engine/buildbucket"},
	{"$recipe_engine/runtime", "is_experimental"},
	{"$recipe_engine/runtime", "is_luci"},
	{"branch"},
	{"buildbucket"},
	{"buildername"},
	{"repository"},
}

// structContains returns whether the struct contains a value at the given path.
// An empty slice of path components always returns true.
func structContains(s *structpb.Struct, path []string) bool {
	for _, p := range path {
		v, ok := s.GetFields()[p]
		if !ok {
			return false
		}
		s = v.GetStructValue()
	}
	return true
}

// validateProperties validates the given properties.
func validateProperties(p *structpb.Struct) error {
	for _, path := range prohibitedProperties {
		if structContains(p, path) {
			return errors.Reason("%q must not be specified", strings.Join(path, ".")).Err()
		}
	}
	return nil
}

type parent struct {
	bld *model.Build
	// ancestors including self.
	ancestors []int64
	pRunID    string
	err       error
}

type parentsMap struct {
	fromRequests map[int64]*parent
	fromToken    *parent
}

// parentForRequest returns the parent of the build to create.
// If the request has ParentBuildId specified,
//   - returns the parent by ParentBuildId,
//   - will panic if missing parentsMap or missing parent from the map.
//
// If the request doesn't specify ParentBuildId, tries to get the parent from
// BUILD token. And missing parentsMap is allowed.
func (ps *parentsMap) parentForRequest(req *pb.ScheduleBuildRequest) *parent {
	if ps == nil {
		if req.GetParentBuildId() != 0 {
			panic(fmt.Sprintf("requested parent %d not found", req.ParentBuildId))
		}
		return nil
	}
	if req.GetParentBuildId() != 0 {
		p, ok := ps.fromRequests[req.ParentBuildId]
		if !ok {
			panic(fmt.Sprintf("requested parent %d not found", req.ParentBuildId))
		}
		return p
	}
	return ps.fromToken
}

func (ps *parentsMap) parentBuildForRequest(req *pb.ScheduleBuildRequest) (*model.Build, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return nil, nil
	case p.err != nil:
		return nil, p.err
	default:
		return p.bld, nil
	}
}

func (ps *parentsMap) ancestorsForRequest(req *pb.ScheduleBuildRequest) ([]int64, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return nil, nil
	case p.err != nil:
		return nil, p.err
	default:
		return p.ancestors, nil
	}
}

func (ps *parentsMap) parentRunIDForRequest(req *pb.ScheduleBuildRequest) (string, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return "", nil
	case p.err != nil:
		return "", p.err
	default:
		return p.pRunID, nil
	}
}

func populateParentFields(p *parent, pBld *model.Build, pInfra *model.BuildInfra) {
	if pBld == nil || pInfra == nil {
		panic("impossible")
	}
	if protoutil.IsEnded(pBld.Proto.GetStatus()) || protoutil.IsEnded(pBld.Proto.GetOutput().GetStatus()) {
		p.err = appstatus.BadRequest(errors.Reason("%d has ended, cannot add child to it", pBld.ID).Err())
		return
	}
	p.bld = pBld
	p.ancestors, p.pRunID = getParentInfo(pBld, pInfra)
}

// validateParentViaToken validates the given parent build, if the request
// contains a BUILD token.
//
// If there is no token present in `ctx`, returns (nil, nil).
// Incorrect tokens, broken tokens, non-BUILD tokens, missing builds, etc.
// all return status-annotated errors.
func validateParentViaToken(ctx context.Context) (p *parent) {
	p = &parent{}
	buildTok, err, hasToken := getBuildbucketToken(ctx, false)
	if errors.Is(err, errBadTokenAuth) {
		if hasToken {
			p.err = err
		}
		return
	}

	// NOTE: We pass buildid == 0 here because we are relying on the token itself
	// to tell us what the parent build ID is. Do not do this in other locations
	// or they will be suceptible to accepting tokens generated for other builds.
	tok, err := buildtoken.ParseToTokenBody(ctx, buildTok, 0, pb.TokenBody_BUILD)
	if err != nil {
		// We don't return `err` here because it will include the Unauthenticated
		// gRPC tag, which isn't accurate.
		p.err = appstatus.BadRequest(errors.New("invalid parent buildbucket token"))
		return
	}

	entities, err := common.GetBuildEntities(ctx, tok.BuildId, model.BuildKind, model.BuildInfraKind)
	if err != nil {
		p.err = err
		return
	}
	populateParentFields(p, entities[0].(*model.Build), entities[1].(*model.BuildInfra))
	return
}

// validateParents validates the given parent build(s).
func validateParents(ctx context.Context, pIDs []int64) (*parentsMap, error) {
	p := validateParentViaToken(ctx)
	switch {
	case p.err != nil:
		return nil, p.err
	case p.bld == nil && len(pIDs) == 0:
		return nil, nil
	case p.bld != nil && len(pIDs) > 0:
		return nil, appstatus.BadRequest(errors.New("parent buildbucket token and parent_build_id are mutually exclusive"))
	case p.bld != nil:
		return &parentsMap{fromToken: p}, nil
	}

	// Get parent builds using pIDs.
	ps := make(map[int64]*parent, len(pIDs))
	blds := make([]*model.Build, len(pIDs))
	infras := make([]*model.BuildInfra, len(pIDs))
	for i, pID := range pIDs {
		ps[pID] = &parent{}
		b := &model.Build{ID: pID}
		blds[i] = b
		infras[i] = &model.BuildInfra{Build: datastore.KeyForObj(ctx, b)}
	}

	asMultiErr := func(err error) errors.MultiError {
		var me errors.MultiError
		if !errors.As(err, &me) {
			me = make(errors.MultiError, len(pIDs))
			for i := range pIDs {
				me[i] = err
			}
		}
		return me
	}

	if err := datastore.Get(ctx, blds, infras); err != nil {
		var merr errors.MultiError
		if !errors.As(err, &merr) {
			logging.Errorf(ctx, "Failed to fetch parent builds %q: %s", pIDs, err)
			return nil, appstatus.Error(codes.Internal, "failed to fetch parent builds")
		}
		bldsMErr := asMultiErr(merr[0])
		infrasMErr := asMultiErr(merr[1])
		mergeErrs(bldsMErr, infrasMErr, "BuildInfra", func(i int) int { return i })
		for i, pID := range pIDs {
			err := bldsMErr[i]
			if err != nil {
				if errors.Is(err, datastore.ErrNoSuchEntity) {
					ps[pID].err = perm.NotFoundErr(ctx)
				} else {
					logging.Errorf(ctx, "failed to fetch parent build %d, %s", pID, err)
					ps[pID].err = appstatus.Errorf(codes.Internal, "failed to fetch parent build %d", pID)
				}
			}
		}
	}

	for i, pID := range pIDs {
		p := ps[pID]
		if p.err != nil {
			continue
		}

		populateParentFields(p, blds[i], infras[i])
	}
	return &parentsMap{fromRequests: ps}, nil
}

// validateSchedule validates the given request.
func validateSchedule(ctx context.Context, req *pb.ScheduleBuildRequest, wellKnownExperiments stringset.Set, parent *model.Build) error {
	var err error
	switch {
	case strings.Contains(req.GetRequestId(), "/"):
		return errors.Reason("request_id cannot contain '/'").Err()
	case req.GetBuilder() == nil && req.GetTemplateBuildId() == 0:
		return errors.Reason("builder or template_build_id is required").Err()
	case req.Builder != nil && teeErr(protoutil.ValidateRequiredBuilderID(req.Builder), &err) != nil:
		return errors.Fmt("builder: %w", err)
	case teeErr(validateRequestedDimensions(req.Dimensions), &err) != nil:
		return errors.Fmt("dimensions: %w", err)
	case teeErr(validateExecutable(req.Exe), &err) != nil:
		return errors.Fmt("exe: %w", err)
	case teeErr(validateGerritChanges(req.GerritChanges), &err) != nil:
		return errors.Fmt("gerrit_changes: %w", err)
	case req.GitilesCommit != nil && teeErr(validateCommitWithRef(req.GitilesCommit), &err) != nil:
		return errors.Fmt("gitiles_commit: %w", err)
	case req.Notify != nil && teeErr(validateNotificationConfig(ctx, req.Notify), &err) != nil:
		return errors.Fmt("notify: %w", err)
	case req.Priority < 0 || req.Priority > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	case req.Properties != nil && teeErr(validateProperties(req.Properties), &err) != nil:
		return errors.Fmt("properties: %w", err)
	case parent == nil && req.CanOutliveParent != pb.Trinary_UNSET:
		return errors.Reason("can_outlive_parent is specified without parent").Err()
	case teeErr(validateTags(req.Tags, TagNew), &err) != nil:
		return errors.Fmt("tags: %w", err)
	}

	for expName := range req.Experiments {
		if err := config.ValidateExperimentName(expName, wellKnownExperiments); err != nil {
			return errors.Annotate(err, "experiment %q", expName).Err()
		}
	}

	// TODO(crbug/1042991): Validate Properties.
	return nil
}

// templateBuildMask enumerates properties to read from template builds. See
// scheduleRequestFromTemplate.
var templateBuildMask = model.HardcodedBuildMask(
	"builder",
	"critical",
	"exe",
	"infra.buildbucket.requested_dimensions",
	"infra.swarming.priority",
	"input.experimental",
	"input.gerrit_changes",
	"input.gitiles_commit",
	"input.properties",
	"tags",
)

func scheduleRequestFromBuildID(ctx context.Context, bID int64, isRetry bool) (*pb.ScheduleBuildRequest, error) {
	bld, err := common.GetBuild(ctx, bID)
	if err != nil {
		return nil, err
	}
	if err := perm.HasInBuilder(ctx, bbperms.BuildsGet, bld.Proto.Builder); err != nil {
		return nil, err
	}

	b := bld.ToSimpleBuildProto(ctx)

	if isRetry && b.Retriable == pb.Trinary_NO {
		return nil, appstatus.BadRequest(errors.Reason("build %d is not retriable", bld.ID).Err())
	}

	if err := model.LoadBuildDetails(ctx, templateBuildMask, nil, b); err != nil {
		return nil, err
	}

	ret := &pb.ScheduleBuildRequest{
		Builder:       b.Builder,
		Critical:      b.Critical,
		Exe:           b.Exe,
		GerritChanges: b.Input.GerritChanges,
		GitilesCommit: b.Input.GitilesCommit,
		Properties:    b.Input.Properties,
		Tags:          b.Tags,
		Dimensions:    b.Infra.GetBuildbucket().GetRequestedDimensions(),
		Priority:      b.Infra.GetSwarming().GetPriority(),
		Retriable:     b.Retriable,
	}

	ret.Experiments = make(map[string]bool, len(bld.Experiments))
	bld.IterExperiments(func(enabled bool, exp string) bool {
		ret.Experiments[exp] = enabled
		return true
	})
	return ret, nil
}

// scheduleRequestFromTemplate returns a request with fields populated by the
// given template_build_id if there is one. Fields set in the request override
// fields populated from the template. Does not modify the incoming request.
func scheduleRequestFromTemplate(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.ScheduleBuildRequest, error) {
	if req.GetTemplateBuildId() == 0 {
		return req, nil
	}

	ret, err := scheduleRequestFromBuildID(ctx, req.TemplateBuildId, true)
	if err != nil {
		return nil, err
	}

	// proto.Merge concatenates repeated fields. Here the desired behavior is replacement,
	// so clear slices from the return value before merging, if specified in the request.
	if req.Exe != nil {
		ret.Exe = nil
	}
	if len(req.GerritChanges) > 0 {
		ret.GerritChanges = nil
	}
	if req.Properties != nil {
		ret.Properties = nil
	}
	if len(req.Tags) > 0 {
		ret.Tags = nil
	}
	if len(req.Dimensions) > 0 {
		ret.Dimensions = nil
	}
	proto.Merge(ret, req)
	ret.TemplateBuildId = 0

	return ret, nil
}

// fetchBuilderConfigs returns the Builder configs referenced by the given
// requests in a map of Bucket ID -> Builder name -> *pb.BuilderConfig,
// a map of buckets to their shadow buckets and a map of Bucket ID -> *pb.Bucket.
//
// A single returned error means a global error which applies to every request.
// Otherwise, it would be a MultiError where len(MultiError) equals to len(builderIDs).
func fetchBuilderConfigs(ctx context.Context, builderIDs []*pb.BuilderID) (map[string]map[string]*pb.BuilderConfig, map[string]*pb.Bucket, map[string]string, error) {
	merr := make(errors.MultiError, len(builderIDs))
	var bcks []*model.Bucket

	// bckCfgs and bldrCfgs use a double-pointer because GetIgnoreMissing will
	// indirectly overwrite the pointer in the model struct when loading from the
	// datastore (so, populating Proto and Config fields and using those values
	// won't help).
	bckCfgs := map[string]**pb.Bucket{} // Bucket ID -> **pb.Bucket
	var bldrs []*model.Builder
	bldrCfgs := map[string]map[string]**pb.BuilderConfig{} // Bucket ID -> Builder name -> **pb.BuilderConfig
	idxMap := map[string]map[string][]int{}                // Bucket ID -> Builder name -> a list of index
	for i, bldr := range builderIDs {
		bucket := protoutil.FormatBucketID(bldr.Project, bldr.Bucket)
		if _, ok := bldrCfgs[bucket]; !ok {
			bldrCfgs[bucket] = make(map[string]**pb.BuilderConfig)
			idxMap[bucket] = map[string][]int{}
		}
		if _, ok := bldrCfgs[bucket][bldr.Builder]; ok {
			idxMap[bucket][bldr.Builder] = append(idxMap[bucket][bldr.Builder], i)
			continue
		}
		if _, ok := bckCfgs[bucket]; !ok {
			b := &model.Bucket{
				Parent: model.ProjectKey(ctx, bldr.Project),
				ID:     bldr.Bucket,
			}
			bckCfgs[bucket] = &b.Proto
			bcks = append(bcks, b)
		}
		b := &model.Builder{
			Parent: model.BucketKey(ctx, bldr.Project, bldr.Bucket),
			ID:     bldr.Builder,
		}
		bldrCfgs[bucket][bldr.Builder] = &b.Config
		bldrs = append(bldrs, b)
		idxMap[bucket][bldr.Builder] = append(idxMap[bucket][bldr.Builder], i)
	}

	// Note; this will fill in bckCfgs and bldrCfgs.
	if err := model.GetIgnoreMissing(ctx, bcks, bldrs); err != nil {
		return nil, nil, nil, errors.Annotate(err, "failed to fetch entities").Err()
	}

	dynamicBuckets := map[string]*pb.Bucket{}
	shadowMap := make(map[string]string)
	// Check buckets to see if they support dynamically scheduling builds for builders which are not pre-defined.
	for _, b := range bcks {
		bucket := protoutil.FormatBucketID(b.Parent.StringID(), b.ID)
		if b.Proto.GetName() == "" {
			for _, bldrIdx := range idxMap[bucket] {
				for idx := range bldrIdx {
					merr[idx] = appstatus.Errorf(codes.NotFound, "bucket not found: %q", b.ID)
				}
			}
		} else {
			shadowMap[bucket] = b.Proto.GetShadow()
		}
	}
	for _, b := range bldrs {
		// Since b.Config isn't a pointer type it will always be non-nil. However, since name is validated
		// as required, it can be used as a proxy for determining whether the builder config was found or
		// not. If it's unspecified, the builder wasn't found. Builds for builders which aren't pre-configured
		// can only be scheduled in buckets which support dynamic builders.
		if b.Config.GetName() == "" {
			bucket := protoutil.FormatBucketID(b.Parent.Parent().StringID(), b.Parent.StringID())
			// TODO(crbug/1042991): Check if bucket is explicitly configured for dynamic builders.
			// Currently buckets do not require pre-defined builders iff they have no Swarming config.
			if (*bckCfgs[bucket]).GetSwarming() == nil {
				delete(bldrCfgs[bucket], b.ID)
				if (*bckCfgs[bucket]).GetDynamicBuilderTemplate() != nil {
					dynamicBuckets[bucket] = *bckCfgs[bucket]
				}
				continue
			}
			for _, idx := range idxMap[bucket][b.ID] {
				merr[idx] = appstatus.Errorf(codes.NotFound, "builder not found: %q", b.ID)
			}
		}
	}

	// deref all the pointers.
	ret := make(map[string]map[string]*pb.BuilderConfig, len(bldrCfgs))
	for bucket, builders := range bldrCfgs {
		m := make(map[string]*pb.BuilderConfig, len(builders))
		for builderName, builder := range builders {
			m[builderName] = *builder
		}
		ret[bucket] = m
	}

	// doesn't contain any errors.
	if merr.First() == nil {
		return ret, dynamicBuckets, shadowMap, nil
	}
	return ret, dynamicBuckets, shadowMap, merr.AsError()
}

// builderMatches returns whether or not the given builder matches the given
// predicate. A match occurs if any regex matches and none of the exclusions
// rule the builder out. If there are no regexes, a match always occurs unless
// an exclusion rules the builder out. The predicate must be validated.
func builderMatches(builder string, pred *pb.BuilderPredicate) bool {
	// TODO(crbug/1042991): Cache compiled regexes (possibly in internal/config package).
	for _, r := range pred.GetRegexExclude() {
		if m, err := regexp.MatchString(fmt.Sprintf("^%s$", r), builder); err == nil && m {
			return false
		}
	}

	if len(pred.GetRegex()) == 0 {
		return true
	}
	for _, r := range pred.Regex {
		if m, err := regexp.MatchString(fmt.Sprintf("^%s$", r), builder); err == nil && m {
			return true
		}
	}
	return false
}

// experimentsMatch returns whether or not the given experimentSet matches the
// given includeOnExperiment or omitOnExperiment.
func experimentsMatch(experimentSet stringset.Set, includeOnExperiment, omitOnExperiment []string) bool {
	for _, e := range omitOnExperiment {
		if experimentSet.Has(e) {
			return false
		}
	}

	if len(includeOnExperiment) > 0 {
		include := false

		for _, e := range includeOnExperiment {
			if experimentSet.Has(e) {
				include = true
				break
			}
		}

		if !include {
			return false
		}

	}

	return true
}

// setDimensions computes the dimensions from the given request and builder
// config, setting them in the proto. Mutates the given *pb.Build.
// build.Infra.Swarming must be set (see setInfra).
func setDimensions(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build, isTaskBackend bool) {
	// Requested dimensions override dimensions specified in the builder config by wiping out all
	// same-key dimensions (regardless of expiration time) in the builder config.
	//
	// For example:
	// Case 1:
	// Request contains: ("key", "value 1", 60), ("key", "value 2", 120)
	// Config contains: ("key", "value 3", 180), ("key", "value 2", 240)
	//
	// Then the result is:
	// ("key", "value 1", 60), ("key", "value 2", 120)
	// Even though the expiration times didn't conflict and theoretically could have been merged.
	//
	// Case 2:
	// Request contains: ("key", "")
	// Config contains: ("key", "value 3", 180), ("key", "value 2", 240)
	//
	// Then all dimensions(Key == "key") are excluded.

	// If the config contains any reference to the builder dimension, ignore its auto builder dimension setting.
	seenBuilder := false

	// key -> slice of dimensions (key, value, expiration) with matching keys.
	dims := make(map[string][]*pb.RequestedDimension)

	// cfg.Dimensions is a slice of strings. Each string has already been validated to match either
	// <key>:<value> or <exp>:<key>:<value>, where <exp> is an int64 expiration time, <key> is a
	// non-empty string which can't be parsed as int64, and <value> is a string which may be empty.
	// <key>:<value> is shorthand for 0:<key>:<value>. An empty <value> means the dimension should be excluded.
	for _, d := range cfg.GetDimensions() {
		exp, k, v := config.ParseDimension(d)
		if k == "builder" {
			seenBuilder = true
		}
		if v == "" {
			// Omit empty <value>.
			continue
		}
		dim := &pb.RequestedDimension{
			Key:   k,
			Value: v,
		}
		if exp > 0 {
			dim.Expiration = &durationpb.Duration{
				Seconds: exp,
			}
		}
		dims[k] = append(dims[k], dim)
	}

	if cfg.GetAutoBuilderDimension() == pb.Toggle_YES && !seenBuilder {
		dims["builder"] = []*pb.RequestedDimension{
			{
				Key:   "builder",
				Value: cfg.GetName(),
			},
		}
	}

	// key -> slice of dimensions (key, value, expiration) with matching keys.
	reqDims := make(map[string][]*pb.RequestedDimension, len(cfg.GetDimensions()))
	for _, d := range req.GetDimensions() {
		if d.GetValue() == "" {
			// Exclude same-key dimensions in the builder config if the dimension
			// value in the request is empty.
			delete(dims, d.Key)
			continue
		}
		reqDims[d.Key] = append(reqDims[d.Key], d)
	}
	for k, d := range reqDims {
		dims[k] = d
	}

	taskDims := make([]*pb.RequestedDimension, 0, len(reqDims))
	for _, d := range dims {
		taskDims = append(taskDims, d...)
	}
	sortRequestedDimension(taskDims)
	if isTaskBackend {
		build.Infra.Backend.TaskDimensions = taskDims
		return
	}
	build.Infra.Swarming.TaskDimensions = taskDims
}

func sortRequestedDimension(dims []*pb.RequestedDimension) {
	sort.Slice(dims, sortby.Chain{
		// Sort by key then expiration.
		func(i, j int) bool { return dims[i].Key < dims[j].Key },
		func(i, j int) bool { return dims[i].Expiration.GetSeconds() < dims[j].Expiration.GetSeconds() },
	}.Use)
}

// setExecutable computes the executable from the given request and builder
// config, setting it in the proto. Mutates the given *pb.Build.
func setExecutable(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	build.Exe = cfg.GetExe()
	if build.Exe == nil {
		build.Exe = &pb.Executable{}
	}

	if cfg.GetRecipe() != nil {
		build.Exe.CipdPackage = cfg.Recipe.CipdPackage
		build.Exe.CipdVersion = cfg.Recipe.CipdVersion
		if build.Exe.CipdVersion == "" {
			build.Exe.CipdVersion = "refs/heads/master"
		}
	}

	// The request has highest precedence, but may only override CIPD version.
	if req.GetExe().GetCipdVersion() != "" {
		build.Exe.CipdVersion = req.Exe.CipdVersion
	}
}

// activeGlobalExpsForBuilder filters the global experiments, returning the
// experiments that apply to this builder, as well as experiments which are
// ignored.
//
// If experiments are known, but don't apply to the builder, then they're
// returned in a form where their DefaultValue and MinimumValue are 0.
//
// Ignored experiments are global experiments which no longer do anything,
// and should be removed from the build (even if specified via
// ScheduleBuildRequest).
func activeGlobalExpsForBuilder(build *pb.Build, globalCfg *pb.SettingsCfg) (active []*pb.ExperimentSettings_Experiment, ignored stringset.Set) {
	exps := globalCfg.GetExperiment().GetExperiments()
	if len(exps) == 0 {
		return nil, nil
	}

	active = make([]*pb.ExperimentSettings_Experiment, 0, len(exps))
	ignored = stringset.New(0)

	bid := protoutil.FormatBuilderID(build.Builder)
	for _, exp := range exps {
		if exp.Inactive {
			ignored.Add(exp.Name)
			continue
		}
		if !builderMatches(bid, exp.Builders) {
			exp = proto.Clone(exp).(*pb.ExperimentSettings_Experiment)
			exp.DefaultValue = 0
			exp.MinimumValue = 0
		}
		active = append(active, exp)
	}

	return
}

// setExperiments computes the experiments from the given request, builder and
// global config, setting them in the proto. Mutates the given *pb.Build.
// build.Infra.Buildbucket, build.Input and build.Exe must not be nil (see
// setInfra, setInput and setExecutable respectively). The request must not set
// legacy experiment values (see normalizeSchedule).
func setExperiments(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, globalCfg *pb.SettingsCfg, build *pb.Build) {
	globalExps, ignoredExps := activeGlobalExpsForBuilder(build, globalCfg)

	// Set up the dice-rolling apparatus
	exps := make(map[string]int32, len(cfg.GetExperiments())+len(globalExps))
	er := make(map[string]pb.BuildInfra_Buildbucket_ExperimentReason, len(exps))

	// 1. Populate with defaults
	for _, exp := range globalExps {
		exps[exp.Name] = exp.DefaultValue
		er[exp.Name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_DEFAULT
	}
	// 2. Overwrite with builder config
	for name, value := range cfg.GetExperiments() {
		er[name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG
		exps[name] = value
	}
	// 3. Overwrite with minimum global experiment values
	for _, exp := range globalExps {
		if exp.MinimumValue > exps[exp.Name] {
			er[exp.Name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_MINIMUM
			exps[exp.Name] = exp.MinimumValue
		}
	}
	// 4. Explicit requests have highest precedence
	for name, enabled := range req.GetExperiments() {
		er[name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
		if enabled {
			exps[name] = 100
		} else {
			exps[name] = 0
		}
	}
	// 5. Remove all inactive global expirements
	ignoredExps.Iter(func(expName string) bool {
		if _, ok := exps[expName]; ok {
			er[expName] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_INACTIVE
			delete(exps, expName)
		}
		return true
	})

	selections := make(map[string]bool, len(exps))

	// Finally, roll the dice. We order `exps` here for test determinisim.
	expNames := make([]string, 0, len(exps))
	for exp := range exps {
		expNames = append(expNames, exp)
	}
	sort.Strings(expNames)
	for _, exp := range expNames {
		pct := exps[exp]
		switch {
		case pct >= 100:
			selections[exp] = true
		case pct <= 0:
			selections[exp] = false
		default:
			selections[exp] = mathrand.Int31n(ctx, 100) < pct
		}
	}

	// For now, continue to set legacy field values from the experiments.
	build.Canary = selections[bb.ExperimentBBCanarySoftware]
	build.Input.Experimental = selections[bb.ExperimentNonProduction]

	// Set experimental values.
	if len(build.Exe.Cmd) > 0 {
		// If the user explicitly set Exe, that counts as a builder
		// configuration.
		er[bb.ExperimentBBAgent] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG

		// If they explicitly picked recipes, this experiment is false.
		// If they explicitly picked luciexe, this experiment is true
		selections[bb.ExperimentBBAgent] = build.Exe.Cmd[0] != "recipes"
	} else if selections[bb.ExperimentBBAgent] {
		// User didn't explicitly set Exe, bbagent was selected
		build.Exe.Cmd = []string{"luciexe"}
	} else {
		// User didn't explicitly set Exe, bbagent was not selected
		build.Exe.Cmd = []string{"recipes"}
	}

	for exp, en := range selections {
		if !en {
			continue
		}
		build.Input.Experiments = append(build.Input.Experiments, exp)
	}
	sort.Strings(build.Input.Experiments)

	if len(er) > 0 {
		build.Infra.Buildbucket.ExperimentReasons = er
	}

	return
}

// defBuilderCacheTimeout is the default value for WaitForWarmCache in the
// pb.BuildInfra_Swarming_CacheEntry whose Name is "builder" (see setInfra).
var defBuilderCacheTimeout = durationpb.New(4 * time.Minute)

// commonCacheToSwarmingCache returns the equivalent
// []*pb.BuildInfra_Swarming_CacheEntry for the given []*pb.CacheEntry.
func commonCacheToSwarmingCache(cache []*pb.CacheEntry) []*pb.BuildInfra_Swarming_CacheEntry {
	var swarmingCache []*pb.BuildInfra_Swarming_CacheEntry
	for _, c := range cache {
		cacheEntry := &pb.BuildInfra_Swarming_CacheEntry{
			EnvVar:           c.GetEnvVar(),
			Name:             c.GetName(),
			Path:             c.GetPath(),
			WaitForWarmCache: c.GetWaitForWarmCache(),
		}
		swarmingCache = append(swarmingCache, cacheEntry)
	}
	return swarmingCache
}

// builderCacheToCommonCache returns the equivalent
// *pb.CacheEntry for the given *pb.BuilderConfig_CacheEntry.
func builderCacheToCommonCache(cache *pb.BuilderConfig_CacheEntry) *pb.CacheEntry {
	if cache == nil {
		return nil
	}
	commonCache := &pb.CacheEntry{
		EnvVar: cache.GetEnvVar(),
		Name:   cache.GetName(),
		Path:   cache.GetPath(),
	}
	if commonCache.Name == "" {
		commonCache.Name = commonCache.Path
	}
	if cache.WaitForWarmCacheSecs > 0 {
		commonCache.WaitForWarmCache = &durationpb.Duration{
			Seconds: int64(cache.WaitForWarmCacheSecs),
		}
	}
	return commonCache
}

// setInfra computes the infra values from the given request and builder config,
// setting them in the proto. Mutates the given *pb.Build. build.Builder must be
// set. Does not set build.Infra.Logdog.Prefix, which can only be determined at
// creation time.
func setInfra(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build, globalCfg *pb.SettingsCfg) {
	appID := info.AppID(ctx) // e.g. cr-buildbucket
	build.Infra = &pb.BuildInfra{
		Bbagent: &pb.BuildInfra_BBAgent{
			CacheDir:    "cache",
			PayloadPath: "kitchen-checkout",
		},
		Buildbucket: &pb.BuildInfra_Buildbucket{
			Hostname:               fmt.Sprintf("%s.appspot.com", appID),
			RequestedDimensions:    req.GetDimensions(),
			RequestedProperties:    req.GetProperties(),
			KnownPublicGerritHosts: globalCfg.GetKnownPublicGerritHosts(),
			BuildNumber:            cfg.GetBuildNumbers() == pb.Toggle_YES,
		},
		Logdog: &pb.BuildInfra_LogDog{
			Hostname: globalCfg.GetLogdog().GetHostname(),
			Project:  build.Builder.GetProject(),
		},
		Resultdb: &pb.BuildInfra_ResultDB{
			Hostname:  globalCfg.GetResultdb().GetHostname(),
			Enable:    cfg.GetResultdb().GetEnable(),
			BqExports: cfg.GetResultdb().GetBqExports(),
		},
	}
	if cfg.GetRecipe() != nil {
		build.Infra.Recipe = &pb.BuildInfra_Recipe{
			CipdPackage: cfg.Recipe.CipdPackage,
			Name:        cfg.Recipe.Name,
		}
	}
}

func setSwarmingOrBackend(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build, globalCfg *pb.SettingsCfg) {
	experiments := stringset.NewFromSlice(build.GetInput().GetExperiments()...)
	// constructing common TaskBackend/Swarming task fields
	priority := int32(cfg.GetPriority())
	if priority == 0 {
		priority = 30
	}
	if req.GetPriority() > 0 {
		priority = req.Priority
	}

	// Request > experimental > proto precedence.
	if experiments.Has(bb.ExperimentNonProduction) && req.GetPriority() == 0 {
		priority = 255
	}
	taskServiceAccount := cfg.GetServiceAccount()

	globalCaches := globalCfg.GetSwarming().GetGlobalCaches()
	taskCaches := make([]*pb.CacheEntry, len(cfg.GetCaches()), len(cfg.GetCaches())+len(globalCaches))
	names := stringset.New(len(cfg.GetCaches()))
	paths := stringset.New(len(cfg.GetCaches()))
	for i, c := range cfg.GetCaches() {
		taskCaches[i] = builderCacheToCommonCache(c)
		names.Add(taskCaches[i].Name)
		paths.Add(taskCaches[i].Path)
	}
	// Requested caches have precedence over global caches.
	// Apply global caches whose names and paths weren't overridden.
	for _, c := range globalCaches {
		if !names.Has(c.GetName()) && !paths.Has(c.GetPath()) {
			taskCaches = append(taskCaches, builderCacheToCommonCache(c))
		}
	}

	if !paths.Has("builder") {
		taskCaches = append(taskCaches, &pb.CacheEntry{
			Name:             fmt.Sprintf("builder_%x_v2", sha256.Sum256([]byte(protoutil.FormatBuilderID(build.Builder)))),
			Path:             "builder",
			WaitForWarmCache: defBuilderCacheTimeout,
		})
	}

	sort.Slice(taskCaches, func(i, j int) bool {
		return taskCaches[i].Path < taskCaches[j].Path
	})
	// Need to configure build.Infra for a backend or swarming.
	isTaskBackend := false
	backendAltExpIsTrue := experiments.Has(bb.ExperimentBackendAlt)
	switch {
	case backendAltExpIsTrue && (cfg.GetBackendAlt() != nil || cfg.GetBackend() != nil):
		cfgToPass := cfg.GetBackend()
		if cfg.GetBackendAlt() != nil {
			cfgToPass = cfg.BackendAlt
		}
		setInfraBackend(ctx, globalCfg, build, cfgToPass, taskCaches, taskServiceAccount, priority, req.GetPriority())
		isTaskBackend = true
	case backendAltExpIsTrue:
		// Derive backend settings using swarming info.
		// This is a temporary solution for raw swarming -> task backend migration,
		// which allows Buildbucket to do the migration behind the scene without
		// any change on builder configs.
		// TODO(crbug.com/1448926): Remove this after the migration is completed and
		// all builder configs are updated with backend/backend_alt configs.
		derivedBackendCfg := deriveBackendCfgFromSwarming(cfg, globalCfg)
		if derivedBackendCfg != nil {
			setInfraBackend(ctx, globalCfg, build, derivedBackendCfg, taskCaches, taskServiceAccount, priority, req.GetPriority())
			isTaskBackend = true
		}
	}
	if !isTaskBackend {
		build.Infra.Swarming = &pb.BuildInfra_Swarming{
			Caches:             commonCacheToSwarmingCache(taskCaches),
			Hostname:           cfg.GetSwarmingHost(),
			ParentRunId:        req.GetSwarming().GetParentRunId(),
			Priority:           priority,
			TaskServiceAccount: taskServiceAccount,
		}
	}

	setDimensions(req, cfg, build, isTaskBackend)
}

func deriveBackendCfgFromSwarming(cfg *pb.BuilderConfig, globalCfg *pb.SettingsCfg) *pb.BuilderConfig_Backend {
	var target string
	for host, backend := range globalCfg.SwarmingBackends {
		if host == cfg.GetSwarmingHost() {
			target = backend
			break
		}
	}
	if target == "" {
		return nil
	}

	return &pb.BuilderConfig_Backend{
		Target: target,
	}
}

// setInput computes the input values from the given request and builder config,
// setting them in the proto. Mutates the given *pb.Build. May panic if the
// builder config is invalid.
func setInput(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	build.Input = &pb.Build_Input{
		Properties: &structpb.Struct{},
	}

	if cfg.GetRecipe() != nil {
		// TODO(crbug/1042991): Deduplicate property parsing logic with config validation for properties.
		build.Input.Properties.Fields = make(map[string]*structpb.Value, len(cfg.Recipe.Properties)+len(cfg.Recipe.PropertiesJ)+1)
		for _, prop := range cfg.Recipe.Properties {
			k, v := strpair.Parse(prop)
			build.Input.Properties.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: v,
				},
			}
		}

		// Values are JSON-encoded strings which need to be unmarshalled to structpb.Struct.
		// jsonpb unmarshals dicts to structpb.Struct, but cannot unmarshal directly to
		// structpb.Value, so create a dummy dict in order to get the structpb.Value.
		// TODO(crbug/1042991): Deduplicate legacy property parsing with buildbucket/cli.
		for _, prop := range cfg.Recipe.PropertiesJ {
			k, v := strpair.Parse(prop)
			s := &structpb.Struct{}
			v = fmt.Sprintf("{\"%s\": %s}", k, v)
			if err := protojson.Unmarshal([]byte(v), s); err != nil {
				// Builder config should have been validated already.
				panic(errors.Annotate(err, "error parsing %q", v).Err())
			}
			build.Input.Properties.Fields[k] = s.Fields[k]
		}
		build.Input.Properties.Fields["recipe"] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: cfg.Recipe.Name,
			},
		}
	} else if cfg.GetProperties() != "" {
		if err := protojson.Unmarshal([]byte(cfg.Properties), build.Input.Properties); err != nil {
			// Builder config should have been validated already.
			panic(errors.Annotate(err, "error unmarshaling builder properties for %q", cfg.GetName()).Err())
		}
	}

	if build.Input.Properties.Fields == nil {
		build.Input.Properties.Fields = make(map[string]*structpb.Value, len(req.GetProperties().GetFields()))
	}

	allowedOverrides := stringset.NewFromSlice(cfg.GetAllowedPropertyOverrides()...)
	anyOverride := allowedOverrides.Has("*")
	for k, v := range req.GetProperties().GetFields() {
		if build.Input.Properties.Fields[k] != nil && !anyOverride && !allowedOverrides.Has(k) {
			logging.Warningf(ctx, "ScheduleBuild: Unpermitted Override for property %q for builder %q (ignored)", k, protoutil.FormatBuilderID(build.Builder))
		}
		build.Input.Properties.Fields[k] = v
	}

	build.Input.GitilesCommit = req.GetGitilesCommit()
	build.Input.GerritChanges = req.GetGerritChanges()
}

// setTags computes the tags from the given request, setting them in the proto.
// Mutates the given *pb.Build.
func setTags(req *pb.ScheduleBuildRequest, build *pb.Build, pRunID string) {
	tags := protoutil.StringPairMap(req.GetTags())
	if req.GetBuilder() != nil {
		tags.Add("builder", req.Builder.Builder)
	}
	if gc := req.GetGitilesCommit(); gc != nil {
		if buildset := protoutil.GitilesBuildSet(gc); buildset != "" {
			tags.Add("buildset", buildset)
		}
		tags.Add("gitiles_ref", gc.Ref)
	}
	for _, ch := range req.GetGerritChanges() {
		tags.Add("buildset", protoutil.GerritBuildSet(ch))
	}
	// Make `parent_task_id` a tag if buildbucket tracks the build's parent/child
	// relationship.
	if len(build.AncestorIds) > 0 {
		// TODO(crbug.com/1031205): Remove this to always use the parent build's
		// task_id to populate the tag.
		if req.GetSwarming().GetParentRunId() != "" {
			tags.Add("parent_task_id", req.Swarming.ParentRunId)
		} else if pRunID != "" {
			tags.Add("parent_task_id", pRunID)
		}
	}
	build.Tags = protoutil.StringPairs(tags)
}

// defGracePeriod is the default value for pb.Build.GracePeriod.
// See setTimeouts.
var defGracePeriod = durationpb.New(30 * time.Second)

// setTimeouts computes the timeouts from the given request and builder config,
// setting them in the proto. Mutates the given *pb.Build.
func setTimeouts(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	// Timeouts in the request have highest precedence, followed by
	// values in the builder config, followed by default values.
	switch {
	case req.GetExecutionTimeout() != nil:
		build.ExecutionTimeout = req.ExecutionTimeout
	case cfg.GetExecutionTimeoutSecs() > 0:
		build.ExecutionTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExecutionTimeoutSecs),
		}
	default:
		build.ExecutionTimeout = durationpb.New(config.DefExecutionTimeout)
	}

	switch {
	case req.GetGracePeriod() != nil:
		build.GracePeriod = req.GracePeriod
	case cfg.GetGracePeriod() != nil:
		build.GracePeriod = cfg.GracePeriod
	default:
		build.GracePeriod = defGracePeriod
	}

	switch {
	case req.GetSchedulingTimeout() != nil:
		build.SchedulingTimeout = req.SchedulingTimeout
	case cfg.GetExpirationSecs() > 0:
		build.SchedulingTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExpirationSecs),
		}
	default:
		build.SchedulingTimeout = durationpb.New(config.DefSchedulingTimeout)
	}
}

// buildFromScheduleRequest returns a build proto created from the given
// request and builder config. Sets fields except those which can only be
// determined at creation time.
//
// TODO(b/371610971): Refactor the code to use a struct to organize the arguments.
func buildFromScheduleRequest(ctx context.Context, req *pb.ScheduleBuildRequest, ancestors []int64, pRunID string, cfg *pb.BuilderConfig, globalCfg *pb.SettingsCfg) (b *pb.Build) {
	b = &pb.Build{
		Builder:         req.Builder,
		Critical:        cfg.GetCritical(),
		WaitForCapacity: cfg.GetWaitForCapacity() == pb.Trinary_YES,
		Retriable:       cfg.GetRetriable(),
	}

	if cfg.GetDescriptionHtml() != "" {
		b.BuilderInfo = &pb.Build_BuilderInfo{
			Description: cfg.GetDescriptionHtml(),
		}
	}

	if req.Critical != pb.Trinary_UNSET {
		b.Critical = req.Critical
	}

	if req.Retriable != pb.Trinary_UNSET {
		b.Retriable = req.Retriable
	}

	if len(ancestors) > 0 {
		b.AncestorIds = ancestors
		// Temporarily accept req.CanOutliveParent to be unset, and treat it
		// the same as pb.Trinary_YES.
		// This is to prevent breakage due to unmatched timelines of deployments
		// (for example recipes rolls and bb CLI rolls).
		// TODO(crbug.com/1031205): after the parent tracking feature is stabled,
		// we should require req.CanOutliveParent to be set.
		b.CanOutliveParent = req.GetCanOutliveParent() != pb.Trinary_NO
	}

	setExecutable(req, cfg, b)
	setInfra(ctx, req, cfg, b, globalCfg) // Requires setExecutable.
	setInput(ctx, req, cfg, b)
	setTags(req, b, pRunID)
	setTimeouts(req, cfg, b)
	setExperiments(ctx, req, cfg, globalCfg, b)         // Requires setExecutable, setInfra, setInput.
	setSwarmingOrBackend(ctx, req, cfg, b, globalCfg)   // Requires setExecutable, setInfra, setInput, setExperiments.
	if err := setInfraAgent(b, globalCfg); err != nil { // Requires setExecutable, setInfra, setExperiments, setSwarmingOrBackend.
		// TODO(crbug.com/1266060) bubble up the error after TaskBackend workflow is ready.
		// The current ScheduleBuild doesn't need this info. Swallow it to not interrupt the normal workflow.
		logging.Warningf(ctx, "Failed to set build.Infra.Buildbucket.Agent for build %d: %s", b.Id, err)
	}
	// Sets the Backend.Config CIPD agent related fields only for swarming task backends
	if b.Infra.Backend != nil && strings.Contains(b.Infra.Backend.Task.Id.Target, "swarming") {
		setInfraBackendConfigAgent(b) // Requires setInfra, setInfraAgent
	}
	return
}

// setInfraAgent populate the agent info from the given settings.
// Mutates the given *pb.Build.
// The build.Builder, build.Canary, build.Exe build.Infra.Buildbucket
// and one of build.Infra.Swarming or build.Infra.Backend must be set.
func setInfraAgent(build *pb.Build, globalCfg *pb.SettingsCfg) error {
	build.Infra.Buildbucket.Agent = &pb.BuildInfra_Buildbucket_Agent{}
	experiments := stringset.NewFromSlice(build.GetInput().GetExperiments()...)
	builderID := protoutil.FormatBuilderID(build.Builder)

	// TODO(crbug.com/1345722) In the future, bbagent will entirely manage the
	// user executable payload, which means Buildbucket should not specify the
	// payload path.
	// We should change the purpose field and use symbolic paths in the input
	// like "$exe" and "$agentUtils".
	// Reference: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3792330/comments/734e18f7_b7f4726d
	build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
		"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
	}

	setInfraAgentInputData(build, globalCfg, experiments, builderID)
	if len(build.Infra.Buildbucket.Agent.Input.Data) > 0 {
		setCipdPackagesCache(build)
	}

	return setInfraAgentSource(build, globalCfg, experiments, builderID)
}

func addInfraAgentInputData(build *pb.Build, builderID, cipdServer, basePath string, experiments stringset.Set, packages []*pb.SwarmingSettings_Package) {
	inputData := build.Infra.Buildbucket.Agent.Input.Data
	purposes := build.Infra.Buildbucket.Agent.Purposes
	for _, p := range packages {
		if !builderMatches(builderID, p.Builders) {
			continue
		}

		if !experimentsMatch(experiments, p.GetIncludeOnExperiment(), p.GetOmitOnExperiment()) {
			continue
		}

		path := basePath
		if p.Subdir != "" {
			path = fmt.Sprintf("%s/%s", path, p.Subdir)
		}
		if _, ok := inputData[path]; !ok {
			inputData[path] = &pb.InputDataRef{
				DataType: &pb.InputDataRef_Cipd{
					Cipd: &pb.InputDataRef_CIPD{
						Server: cipdServer,
					},
				},
				OnPath: []string{path, fmt.Sprintf("%s/%s", path, "bin")},
			}
			if basePath == BbagentUtilPkgDir {
				purposes[path] = pb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY
			}
		}

		inputData[path].GetCipd().Specs = append(inputData[path].GetCipd().Specs, &pb.InputDataRef_CIPD_PkgSpec{
			Package: p.PackageName,
			Version: extractCipdVersion(p, build),
		})
	}
}

// setInfraAgentInputData populate input cipd info from the given settings.
// In the future, they can be also from per-builder-level or per-request-level.
// Mutates the given *pb.Build.
// The build.Builder, build.Canary, build.Exe, and build.Infra.Buildbucket.Agent must be set
func setInfraAgentInputData(build *pb.Build, globalCfg *pb.SettingsCfg, experiments stringset.Set, builderID string) {
	inputData := make(map[string]*pb.InputDataRef)
	build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
		Data: inputData,
	}

	// add cipd client.
	cipdServer := globalCfg.GetCipd().GetServer()
	version := globalCfg.GetCipd().GetSource().GetVersion()
	if build.Canary && globalCfg.GetCipd().GetSource().GetVersionCanary() != "" {
		version = globalCfg.GetCipd().GetSource().GetVersionCanary()
	}
	if version != "" {
		build.Infra.Buildbucket.Agent.Input.CipdSource = map[string]*pb.InputDataRef{
			CipdClientDir: {
				DataType: &pb.InputDataRef_Cipd{
					Cipd: &pb.InputDataRef_CIPD{
						Server: cipdServer,
						Specs: []*pb.InputDataRef_CIPD_PkgSpec{
							{
								Package: globalCfg.GetCipd().GetSource().GetPackageName(),
								Version: version,
							},
						},
					},
				},
				OnPath: []string{CipdClientDir, fmt.Sprintf("%s/%s", CipdClientDir, "bin")},
			},
		}
		build.Infra.Buildbucket.Agent.CipdClientCache = &pb.CacheEntry{
			// Sha the version to make sure the cache name matches
			// "^[a-z0-9_]{1,4096}$".
			Name: fmt.Sprintf("cipd_client_%x", sha256.Sum256([]byte(version))),
			Path: "cipd_client",
		}
	}

	// add user packages.
	addInfraAgentInputData(build, builderID, cipdServer, UserPackageDir, experiments, globalCfg.GetSwarming().GetUserPackages())

	// add bbagent utility packages.
	addInfraAgentInputData(build, builderID, cipdServer, BbagentUtilPkgDir, experiments, globalCfg.GetSwarming().GetBbagentUtilityPackages())

	if build.Exe.GetCipdPackage() != "" || build.Exe.GetCipdVersion() != "" {
		inputData["kitchen-checkout"] = &pb.InputDataRef{
			DataType: &pb.InputDataRef_Cipd{
				Cipd: &pb.InputDataRef_CIPD{
					Server: cipdServer,
					Specs: []*pb.InputDataRef_CIPD_PkgSpec{
						{
							Package: build.Exe.GetCipdPackage(),
							Version: build.Exe.GetCipdVersion(),
						},
					},
				},
			},
		}
	}
}

// setInfraAgentSource extracts bbagent source info from the given settings.
// In the future, they can be also from per-builder-level or per-request-level.
// Mutates the given *pb.Build.
// The build.Canary, build.Infra.Buildbucket.Agent must be set
func setInfraAgentSource(build *pb.Build, globalCfg *pb.SettingsCfg, experiments stringset.Set, builderID string) error {
	bbagent := globalCfg.GetSwarming().GetBbagentPackage()
	bbagentAlternatives := make([]*pb.SwarmingSettings_Package, 0, len(globalCfg.GetSwarming().GetAlternativeAgentPackages()))
	for _, p := range globalCfg.GetSwarming().GetAlternativeAgentPackages() {
		if !builderMatches(builderID, p.Builders) {
			continue
		}

		if !experimentsMatch(experiments, p.GetIncludeOnExperiment(), p.GetOmitOnExperiment()) {
			continue
		}

		bbagentAlternatives = append(bbagentAlternatives, p)
	}
	if len(bbagentAlternatives) > 1 {
		return errors.Reason("cannot decide buildbucket agent source").Err()
	}
	if len(bbagentAlternatives) == 1 {
		bbagent = bbagentAlternatives[0]
	}
	if bbagent == nil {
		return nil
	}

	if !strings.HasSuffix(bbagent.PackageName, "/${platform}") {
		return errors.New("bad settings: bbagent package name must end with '/${platform}'")
	}
	cipdHost := globalCfg.GetCipd().GetServer()
	build.Infra.Buildbucket.Agent.Source = &pb.BuildInfra_Buildbucket_Agent_Source{
		DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
			Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
				Package: bbagent.PackageName,
				Version: extractCipdVersion(bbagent, build),
				Server:  cipdHost,
			},
		},
	}
	return nil
}

// setInfraBackendConfigAgent extracts bbagent source info from the build proto.
// Mutates the given *pb.Build.
// The build.Infra.Buildbucket.Agent must be set
func setInfraBackendConfigAgent(b *pb.Build) {
	agentSource := b.Infra.Buildbucket.GetAgent().GetSource()
	b.Infra.Backend.Config.Fields["agent_binary_cipd_pkg"] = structpb.NewStringValue(agentSource.GetCipd().Package)
	b.Infra.Backend.Config.Fields["agent_binary_cipd_vers"] = structpb.NewStringValue(agentSource.GetCipd().Version)
	b.Infra.Backend.Config.Fields["agent_binary_cipd_server"] = structpb.NewStringValue(buildURL(agentSource.GetCipd().Server))
	// TODO(crbug.com/1420443): Remove this harcoding and use
	// globalCfg.GetSwarming().GetBbagentPackage().binary_agent_name.
	b.Infra.Backend.Config.Fields["agent_binary_cipd_filename"] = structpb.NewStringValue("bbagent${EXECUTABLE_SUFFIX}")
}

func setInfraBackend(ctx context.Context, globalCfg *pb.SettingsCfg, build *pb.Build, backend *pb.BuilderConfig_Backend, taskCaches []*pb.CacheEntry, taskServiceAccount string, priority, reqPriority int32) {
	config := &structpb.Struct{}
	if backend.GetConfigJson() != "" { // bypass empty config_json
		err := json.Unmarshal([]byte(backend.ConfigJson), config)
		if err != nil {
			logging.Warningf(ctx, err.Error())
		}
	}
	if config.GetFields() == nil {
		config.Fields = make(map[string]*structpb.Value)
	}

	if config.Fields["service_account"].GetStringValue() == "" && taskServiceAccount != "" {
		config.Fields["service_account"] = structpb.NewStringValue(taskServiceAccount)
	}

	// If request has a priority, use that
	// else if backend config_json did not have a priority
	// we use the builder one (or value 30 if builder was not set)
	if config.Fields["priority"].GetNumberValue() == 0 || reqPriority > 0 {
		config.Fields["priority"] = structpb.NewNumberValue(float64(priority))
	}
	hostname, err := clients.GetBackendHost(backend.GetTarget(), globalCfg)
	if err != nil {
		logging.Warningf(ctx, err.Error())
	}

	if build.WaitForCapacity {
		config.Fields["wait_for_capacity"] = structpb.NewBoolValue(true)
	}

	for _, c := range taskCaches {
		c.Path = fmt.Sprintf("%s/%s", build.Infra.Bbagent.CacheDir, c.Path)
	}

	build.Infra.Backend = &pb.BuildInfra_Backend{
		Caches: taskCaches,
		Config: config,
		Task: &pb.Task{
			Id: &pb.TaskID{
				Target: backend.GetTarget(),
			},
			UpdateId: 0,
		},
		Hostname: hostname,
	}
}

// setExperimentsFromProto sets experiments in the model (see model/build.go).
// build.Proto.Input.Experiments and
// build.Proto.Infra.Buildbucket.ExperimentReasons must be set (see setExperiments).
func setExperimentsFromProto(build *model.Build) {
	setExps := stringset.NewFromSlice(build.Proto.Input.Experiments...)
	for exp := range build.Proto.Infra.Buildbucket.ExperimentReasons {
		if !setExps.Has(exp) {
			build.Experiments = append(build.Experiments, fmt.Sprintf("-%s", exp))
		}
	}
	for _, exp := range build.Proto.Input.Experiments {
		build.Experiments = append(build.Experiments, fmt.Sprintf("+%s", exp))
	}
	sort.Strings(build.Experiments)

	build.Canary = build.Proto.Canary
	build.Experimental = build.Proto.Input.Experimental
}

func getParentInfo(pBld *model.Build, pInfra *model.BuildInfra) (ancestors []int64, pRunID string) {
	switch {
	case pBld == nil:
		ancestors = make([]int64, 0)
	case len(pBld.AncestorIds) > 0:
		ancestors = append(pBld.AncestorIds, pBld.ID)
	default:
		ancestors = append(ancestors, pBld.ID)
	}

	if pBld != nil && pInfra != nil {
		pTaskID := pInfra.Proto.GetBackend().GetTask().GetId()
		pTarget := pTaskID.GetTarget()
		if !strings.HasPrefix(pTarget, "swarming://") {
			return
		}
		pRunID = pTaskID.GetId()
		if pRunID != "" {
			pRunID = pRunID[:len(pRunID)-1] + "1"
		}
	}
	return
}

// getShadowBuckets gets the shadow buckets.
//
// For the requests with `ShadowInput`, the build should be scheduled in the
// shadow bucket of the requested bucket. So we need to get the shadow buckets
// for validation.
func getShadowBuckets(ctx context.Context, reqs []*pb.ScheduleBuildRequest) (map[string]string, error) {
	bcksWithShadow := stringset.New(0)
	var buckets []*model.Bucket
	for _, req := range reqs {
		if req.GetShadowInput() == nil {
			continue
		}
		k := protoutil.FormatBucketID(req.Builder.Project, req.Builder.Bucket)
		if bcksWithShadow.Add(k) {
			buckets = append(buckets, &model.Bucket{
				Parent: model.ProjectKey(ctx, req.Builder.Project),
				ID:     req.Builder.Bucket,
			})
		}
	}
	if len(bcksWithShadow) == 0 {
		return nil, nil
	}

	if err := model.GetIgnoreMissing(ctx, buckets); err != nil {
		return nil, errors.Annotate(err, "failed to fetch bucket entities").Err()
	}

	shadows := make(map[string]string)
	for _, b := range buckets {
		if b == nil {
			continue
		}
		k := protoutil.FormatBucketID(b.Parent.StringID(), b.ID)
		shadows[k] = b.Proto.GetShadow()
	}
	return shadows, nil
}

func builderCustomMetrics(ctx context.Context, globalCfg *pb.SettingsCfg, cfg *pb.BuilderConfig) []model.CustomMetric {
	if len(cfg.GetCustomMetricDefinitions()) == 0 {
		return nil
	}

	gms := make(map[string]pb.CustomMetricBase, len(globalCfg.GetCustomMetrics()))
	for _, gm := range globalCfg.GetCustomMetrics() {
		gms[gm.Name] = gm.GetMetricBase()
	}

	cms := make([]model.CustomMetric, 0, len(cfg.CustomMetricDefinitions))
	for _, bcm := range cfg.CustomMetricDefinitions {
		base, ok := gms[bcm.Name]
		if !ok {
			// Was the metric removed from our global settings?
			logging.Warningf(ctx, "metric %s for builder %s not found in global settings", bcm.Name, cfg.Name)
			continue
		}
		cms = append(cms, model.CustomMetric{
			Base:   base,
			Metric: bcm,
		})
	}
	return cms
}

// scheduleBuilds handles requests to schedule builds. Requests must be validated and authorized.
// The length of returned builds always equal to len(reqs).
// A single returned error means a global error which applies to every request.
// Otherwise, it would be a MultiError where len(MultiError) equals to len(reqs).
func scheduleBuilds(ctx context.Context, globalCfg *pb.SettingsCfg, pMap *parentsMap, reqs ...*pb.ScheduleBuildRequest) ([]*model.Build, error) {
	if len(reqs) == 0 {
		return []*model.Build{}, nil
	}

	dryRun := reqs[0].DryRun
	for _, req := range reqs {
		if req.DryRun != dryRun {
			return nil, appstatus.BadRequest(errors.Reason("all requests must have the same dry_run value").Err())
		}
	}

	merr := make(errors.MultiError, len(reqs))
	// Bucket -> Builder -> *pb.BuilderConfig.
	bldrIDs := make([]*pb.BuilderID, 0, len(reqs))
	for _, req := range reqs {
		bldrIDs = append(bldrIDs, req.Builder)
	}
	cfgs, dynamicBuckets, shadowMap, err := fetchBuilderConfigs(ctx, bldrIDs)
	if me, ok := err.(errors.MultiError); ok {
		merr = mergeErrs(merr, me, "error fetching builders", func(i int) int { return i })
	} else if err != nil {
		return nil, err
	}

	validReq, idxMapBlds := getValidReqs(reqs, merr)
	blds := make([]*model.Build, len(validReq))
	resultdbOpts := make([]resultdb.CreateOptions, len(validReq))
	bldrsMCB := stringset.New(0)

	var pInfra *model.BuildInfra
	for i := range blds {
		origI := idxMapBlds[i]
		pBld, err := pMap.parentBuildForRequest(reqs[origI])
		if err != nil {
			merr[origI] = errors.Reason("error getting the parent").Err()
			blds[i] = nil
			continue
		}
		// Any error here would have been caught by pMap.parentBuildForRequest
		ancestors, _ := pMap.ancestorsForRequest(reqs[origI])
		pRunID, _ := pMap.parentRunIDForRequest(reqs[origI])

		bucket := fmt.Sprintf("%s/%s", validReq[i].Builder.Project, validReq[i].Builder.Bucket)
		cfg := cfgs[bucket][validReq[i].Builder.Builder]
		inDynamicBucket := false
		if bkt, ok := dynamicBuckets[bucket]; ok {
			inDynamicBucket = true
			cfg = bkt.GetDynamicBuilderTemplate().GetTemplate()
		}

		var build *pb.Build
		if reqs[origI].ShadowInput != nil {
			// Schedule a build with shadow info.
			if shadowMap[bucket] == "" || shadowMap[bucket] == validReq[i].Builder.Bucket {
				// Scheduling a shadow build in the original bucket is prohibited.
				// In theory this part of code should not be reached, since validateScheduleBuild
				// has checked.
				// But still check here just in case a builder config happened to be
				// updated between validateScheduleBuild and here.
				merr[origI] = errors.Reason("scheduling a shadow build in the original bucket is not allowed").Err()
				blds[i] = nil
				continue
			}
			// Schedule a build with shadow info.
			build = scheduleShadowBuild(ctx, reqs[origI], ancestors, shadowMap[bucket], globalCfg, cfg)
			if pBld != nil {
				if pInfra == nil {
					entities, err := common.GetBuildEntities(ctx, pBld.ID, model.BuildInfraKind)
					if err != nil {
						merr[origI] = errors.Reason("failed to get BuildInfra for build %d", pBld.ID).Err()
						blds[i] = nil
						continue
					}
					pInfra = entities[0].(*model.BuildInfra)
				}
				// Inherit agent input and agent source from the parent build.
				if reqs[origI].ShadowInput.InheritFromParent {
					build.Infra.Buildbucket.Agent.Input = pInfra.Proto.Buildbucket.Agent.Input
					build.Infra.Buildbucket.Agent.Source = pInfra.Proto.Buildbucket.Agent.Source
					build.Exe = pBld.Proto.Exe
					if len(build.Infra.Buildbucket.Agent.Input.Data) > 0 {
						setCipdPackagesCache(build)
					}
				}
			}
		} else {
			// TODO(crbug.com/1042991): Parallelize build creation from requests if necessary.
			build = buildFromScheduleRequest(ctx, reqs[origI], ancestors, pRunID, cfg, globalCfg)
		}

		blds[i] = &model.Build{
			Proto: build,
		}
		// max_concurrent_builds is enabled for this builder.
		if cfg.GetMaxConcurrentBuilds() > 0 {
			bldrsMCB.Add(protoutil.FormatBuilderID(blds[i].Proto.Builder))
		}
		resultdbOpts[i] = resultdb.CreateOptions{
			// Build is an export root in ResultDB if it has no parent, or if
			// explicitly requested.
			IsExportRoot: pBld == nil || validReq[i].GetResultdb().GetIsExportRootOverride(),
		}

		setExperimentsFromProto(blds[i])
		blds[i].IsLuci = cfg != nil || inDynamicBucket
		blds[i].PubSubCallback.Topic = validReq[i].GetNotify().GetPubsubTopic()
		blds[i].PubSubCallback.UserData = validReq[i].GetNotify().GetUserData()
		// Tags are stored in the outer struct (see model/build.go).
		tags := protoutil.StringPairMap(blds[i].Proto.Tags).Format()
		tags = stringset.NewFromSlice(tags...).ToSlice() // Deduplicate tags.
		sort.Strings(tags)
		blds[i].Tags = tags
		blds[i].CustomMetrics = builderCustomMetrics(ctx, globalCfg, cfg)

		exp := make(map[int64]struct{})
		for _, d := range blds[i].Proto.Infra.GetSwarming().GetTaskDimensions() {
			exp[d.Expiration.GetSeconds()] = struct{}{}
		}
		if len(exp) > 6 {
			merr[origI] = appstatus.BadRequest(errors.Reason("build %d contains more than 6 unique expirations", i).Err())
			continue
		}
	}
	if dryRun {
		if merr.First() == nil {
			return blds, nil
		}
		return blds, merr
	}

	reqIDs := make([]string, 0, len(reqs))
	for _, req := range reqs {
		reqIDs = append(reqIDs, req.RequestId)
	}
	bc := &buildCreator{
		blds:           blds,
		resultdbOpts:   resultdbOpts,
		idxMapBldToReq: idxMapBlds,
		reqIDs:         reqIDs,
		merr:           merr,
		bldrsMCB:       bldrsMCB,
	}
	return bc.createBuilds(ctx)
}

// normalizeSchedule converts deprecated fields to non-deprecated ones.
//
// In particular, this currently converts the Canary and Experimental fields to
// the non-deprecated Experiments field.
func normalizeSchedule(req *pb.ScheduleBuildRequest) {
	if req.Experiments == nil {
		req.Experiments = map[string]bool{}
	}

	if _, has := req.Experiments[bb.ExperimentBBCanarySoftware]; !has {
		if req.Canary == pb.Trinary_YES {
			req.Experiments[bb.ExperimentBBCanarySoftware] = true
		} else if req.Canary == pb.Trinary_NO {
			req.Experiments[bb.ExperimentBBCanarySoftware] = false
		}
		req.Canary = pb.Trinary_UNSET
	}

	if _, has := req.Experiments[bb.ExperimentNonProduction]; !has {
		if req.Experimental == pb.Trinary_YES {
			req.Experiments[bb.ExperimentNonProduction] = true
		} else if req.Experimental == pb.Trinary_NO {
			req.Experiments[bb.ExperimentNonProduction] = false
		}
		req.Experimental = pb.Trinary_UNSET
	}
}

// validateScheduleBuild validates and authorizes the given request, returning
// a normalized version of the request and field mask.
func validateScheduleBuild(ctx context.Context, wellKnownExperiments stringset.Set, req *pb.ScheduleBuildRequest, pMap *parentsMap, shadowBuckets map[string]string) (*pb.ScheduleBuildRequest, *model.BuildMask, error) {
	pBld, err := pMap.parentBuildForRequest(req)
	if err != nil {
		return nil, nil, err
	}
	if err = validateSchedule(ctx, req, wellKnownExperiments, pBld); err != nil {
		return nil, nil, appstatus.BadRequest(err)
	}
	normalizeSchedule(req)

	m, err := model.NewBuildMask("", req.Fields, req.Mask)
	if err != nil {
		return nil, nil, appstatus.BadRequest(errors.Annotate(err, "invalid mask").Err())
	}

	if req, err = scheduleRequestFromTemplate(ctx, req); err != nil {
		return nil, nil, err
	}

	bkt := req.Builder.Bucket
	if req.GetShadowInput() != nil {
		k := protoutil.FormatBucketID(req.Builder.Project, req.Builder.Bucket)
		shadow := shadowBuckets[k]
		if shadow == "" || shadow == req.Builder.Bucket {
			return nil, nil, appstatus.BadRequest(errors.Reason("scheduling a shadow build in the original bucket is not allowed").Err())
		}
		bkt = shadow
	}

	if err = perm.HasInBucket(ctx, bbperms.BuildsAdd, req.Builder.Project, bkt); err != nil {
		return nil, nil, err
	}
	if req.ParentBuildId != 0 {
		if err = perm.HasInBucket(ctx, bbperms.BuildsAddAsChild, req.Builder.Project, bkt); err != nil {
			return nil, nil, err
		}
		if err = perm.HasInBuilder(ctx, bbperms.BuildsIncludeChild, pBld.Proto.Builder); err != nil {
			return nil, nil, err
		}
	}
	return req, m, nil
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching service config").Err()
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	var pIDs []int64
	if req.ParentBuildId > 0 {
		pIDs = append(pIDs, req.ParentBuildId)
	}
	pMap, err := validateParents(ctx, pIDs)
	if err != nil {
		return nil, err
	}

	// get shadow buckets.
	shadowBuckets, err := getShadowBuckets(ctx, []*pb.ScheduleBuildRequest{req})
	if err != nil {
		return nil, errors.Annotate(err, "error in getting shadow buckets").Err()
	}

	req, m, err := validateScheduleBuild(ctx, wellKnownExperiments, req, pMap, shadowBuckets)
	if err != nil {
		return nil, err
	}

	blds, err := scheduleBuilds(ctx, globalCfg, pMap, req)
	if err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			return nil, merr.First()
		}
		return nil, err
	}
	if req.DryRun {
		// Dry run build is not saved in datastore, return the proto right away.
		return blds[0].Proto, nil
	}

	// No need to redact the response here, because we're effectively just sending
	// the caller's inputs back to them.
	return blds[0].ToProto(ctx, m, nil)
}

// scheduleBuilds handles requests to schedule builds.
// The length of returned builds and errors (if any) always equal to the len(reqs).
// The returned error type is always MultiError.
func (*Builds) scheduleBuilds(ctx context.Context, globalCfg *pb.SettingsCfg, reqs []*pb.ScheduleBuildRequest) ([]*pb.Build, errors.MultiError) {
	// The ith error is the error associated with the ith request.
	merr := make(errors.MultiError, len(reqs))
	// The ith mask is the field mask derived from the ith request.
	masks := make([]*model.BuildMask, len(reqs))
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	errorInBatch := func(err error, attach func(error) error) errors.MultiError {
		for i, e := range merr {
			if e == nil {
				merr[i] = attach(err)
			}
		}
		return merr
	}

	// Validate parents.
	pIDs := make([]int64, 0, len(reqs))
	pIDSet := make(map[int64]struct{})
	for _, req := range reqs {
		if req.ParentBuildId == 0 {
			continue
		}
		if _, ok := pIDSet[req.ParentBuildId]; !ok {
			pIDSet[req.ParentBuildId] = struct{}{}
			pIDs = append(pIDs, req.ParentBuildId)
		}
	}
	pMap, err := validateParents(ctx, pIDs)
	if err != nil {
		return nil, errorInBatch(err, func(err error) error {
			return errors.Annotate(err, "error in schedule batch").Err()
		})
	}

	// get shadow buckets.
	shadowBuckets, err := getShadowBuckets(ctx, reqs)
	if err != nil {
		return nil, errorInBatch(err, func(err error) error {
			return appstatus.BadRequest(errors.Annotate(err, "error in schedule batch").Err())
		})
	}

	// Validate requests.
	_ = parallel.WorkPool(min(64, len(reqs)), func(work chan<- func() error) {
		for i, req := range reqs {
			work <- func() error {
				reqs[i], masks[i], merr[i] = validateScheduleBuild(ctx, wellKnownExperiments, req, pMap, shadowBuckets)
				return nil
			}
		}
	})

	validReqs, idxMapValidReqs := getValidReqs(reqs, merr)
	// Non-MultiError error should apply to every item and fail all requests.
	blds, err := scheduleBuilds(ctx, globalCfg, pMap, validReqs...)
	if err != nil {
		if me, ok := err.(errors.MultiError); ok {
			merr = mergeErrs(merr, me, "", func(i int) int { return idxMapValidReqs[i] })
		} else {
			return nil, errorInBatch(err, func(err error) error {
				if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
					return err
				} else {
					return appstatus.Errorf(codes.Internal, "error in schedule batch: %s", err)
				}
			})
		}
	}

	ret := make([]*pb.Build, len(blds))
	_ = parallel.WorkPool(min(64, len(blds)), func(work chan<- func() error) {
		for i, bld := range blds {
			if bld == nil {
				continue
			}
			origI := idxMapValidReqs[i]
			work <- func() error {
				// Note: We don't redact the Build response here because we expect any user with
				// BuildsAdd permission should also have BuildsGet.
				// TODO(crbug/1042991): Don't re-read freshly written entities (see ToProto).
				ret[i], merr[origI] = bld.ToProto(ctx, masks[origI], nil)
				return nil
			}
		}
	})

	if merr.First() == nil {
		return ret, nil
	}
	origRet := make([]*pb.Build, len(reqs))
	for i, origI := range idxMapValidReqs {
		if merr[origI] == nil {
			origRet[origI] = ret[i]
		}
	}
	return origRet, merr
}

// mergeErrs merges errs into origErrs according to the idxMapper.
func mergeErrs(origErrs, errs errors.MultiError, reason string, idxMapper func(int) int) errors.MultiError {
	for i, err := range errs {
		if err != nil {
			origErrs[idxMapper(i)] = errors.Annotate(err, reason).Err()
		}
	}
	return origErrs
}

// getValidReqs returns a list of valid ScheduleBuildRequest where its corresponding error is nil,
// as well as an index map where idxMap[returnedIndex] == originalIndex.
func getValidReqs(reqs []*pb.ScheduleBuildRequest, errs errors.MultiError) ([]*pb.ScheduleBuildRequest, []int) {
	if len(reqs) != len(errs) {
		panic("The length of reqs and the length of errs must be the same.")
	}
	var validReqs []*pb.ScheduleBuildRequest
	var idxMap []int
	for i, req := range reqs {
		if errs[i] == nil {
			idxMap = append(idxMap, i)
			validReqs = append(validReqs, req)
		}
	}
	return validReqs, idxMap
}

func extractCipdVersion(p *pb.SwarmingSettings_Package, b *pb.Build) string {
	if b.Canary && p.VersionCanary != "" {
		return p.VersionCanary
	}
	return p.Version
}

// setCipdPackagesCache sets the named cache for bbagent downloaded cipd packages.
// One of build.Infra.Swarming and build.Infra.Backend must be set.
func setCipdPackagesCache(build *pb.Build) {
	var taskServiceAccount string
	if build.Infra.Swarming != nil {
		taskServiceAccount = build.Infra.Swarming.TaskServiceAccount
	} else if build.Infra.Backend.GetConfig() != nil {
		taskServiceAccount = build.Infra.Backend.Config.Fields["service_account"].GetStringValue()
	}
	build.Infra.Buildbucket.Agent.CipdPackagesCache = &pb.CacheEntry{
		Name: fmt.Sprintf("cipd_cache_%x", sha256.Sum256([]byte(taskServiceAccount))),
		Path: "cipd_cache",
	}
}

func buildURL(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("invalid base URL: %s", err))
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return u.String()
}
