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

package recorder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"mime"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/bqutil"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/util"
)

// TODO(crbug.com/1177213) - make this configurable.
const MaxBatchCreateArtifactSize = 10 * 1024 * 1024

// MaxShardContentSize is the maximum content size in BQ row.
// Artifacts content bigger than this size needs to be sharded.
// Leave 10 KB for other fields, the rest is content.
const MaxShardContentSize = bqutil.RowMaxBytes - 10*1024

// LookbackWindow is used when chunking. It specifies how many bytes we should
// look back to find new line/white space characters to split the chunks.
const LookbackWindow = 1024

var (
	artifactExportCounter = metric.NewCounter(
		"resultdb/artifacts/bqexport",
		"The number of artifacts rows to export to BigQuery, grouped by project and status.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the export.
		// Possible values:
		// - "success": The export was successful.
		// - "failure_input": There was an error with the input artifact
		// (e.g. artifact contains invalid UTF-8 character).
		// - "failure_bq": There was an error with BigQuery (e.g. throttling, load shedding),
		// which made the artifact failed to export.
		field.String("status"),
	)

	artifactContentCounter = metric.NewCounter(
		"resultdb/artifacts/content",
		"The number of artifacts for a particular content type.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the export.
		// Possible values: "text", "nontext", "empty".
		// We record the group instead of the actual value to prevent
		// the explosion in cardinality.
		field.String("content_type"),
	)
)

type artifactCreationRequest struct {
	testID      string
	resultID    string
	artifactID  string
	contentType string

	// hash is a hash of the artifact data.  It is not supplied or calculated for GCS artifacts.
	hash string
	// size is the size of the artifact data in bytes.  In the case of a GCS artifact it is user-specified, optional and not verified.
	size int64
	// data is the artifact contents data that will be stored in RBE-CAS.  If gcsURI is provided, this must be empty.
	data []byte
	// gcsURI is the location of the artifact content if it is stored in GCS.  If this is provided, data must be empty.
	gcsURI string
	// status of the artifact's parent test result.
	testStatus pb.TestStatus
}

type invocationInfo struct {
	id         string
	realm      string
	createTime time.Time
}

// BQExportClient is the interface for exporting artifacts.
type BQExportClient interface {
	InsertArtifactRows(ctx context.Context, rows []*bqpb.TextArtifactRow) error
}

// name returns the artifact name.
func (a *artifactCreationRequest) name(invID invocations.ID) string {
	if a.testID == "" {
		return pbutil.InvocationArtifactName(string(invID), a.artifactID)
	}
	return pbutil.TestResultArtifactName(string(invID), a.testID, a.resultID, a.artifactID)
}

// parentID returns the local parent ID of the artifact.
func (a *artifactCreationRequest) parentID() string {
	return artifacts.ParentID(a.testID, a.resultID)
}

func parseCreateArtifactRequest(req *pb.CreateArtifactRequest) (invocations.ID, *artifactCreationRequest, error) {
	if req.GetArtifact() == nil {
		return "", nil, errors.Reason("artifact: unspecified").Err()
	}
	if err := pbutil.ValidateArtifactID(req.Artifact.ArtifactId); err != nil {
		return "", nil, errors.Annotate(err, "artifact_id").Err()
	}
	if req.Artifact.ContentType != "" {
		if _, _, err := mime.ParseMediaType(req.Artifact.ContentType); err != nil {
			return "", nil, errors.Annotate(err, "content_type").Err()
		}
	}

	// parent
	if req.Parent == "" {
		return "", nil, errors.Reason("parent: unspecified").Err()
	}
	invIDStr, testID, resultID, err := pbutil.ParseTestResultName(req.Parent)
	if err != nil {
		if invIDStr, err = pbutil.ParseInvocationName(req.Parent); err != nil {
			return "", nil, errors.Reason("parent: neither valid invocation name nor valid test result name").Err()
		}
	}

	if len(req.Artifact.Contents) != 0 && req.Artifact.GcsUri != "" {
		return "", nil, errors.Reason("only one of contents and gcs_uri can be given").Err()
	}

	sizeBytes := int64(len(req.Artifact.Contents))

	if sizeBytes != 0 && req.Artifact.SizeBytes != 0 && sizeBytes != req.Artifact.SizeBytes {
		return "", nil, errors.Reason("sizeBytes and contents are specified but don't match").Err()
	}

	// If contents field is empty, try to set size from the request instead.
	if sizeBytes == 0 {
		if req.Artifact.SizeBytes != 0 {
			sizeBytes = req.Artifact.SizeBytes
		}
	}

	return invocations.ID(invIDStr), &artifactCreationRequest{
		artifactID:  req.Artifact.ArtifactId,
		contentType: req.Artifact.ContentType,
		data:        req.Artifact.Contents,
		size:        sizeBytes,
		testID:      testID,
		resultID:    resultID,
		gcsURI:      req.Artifact.GcsUri,
		testStatus:  req.Artifact.TestStatus,
	}, nil
}

// parseBatchCreateArtifactsRequest parses a batch request and returns
// artifactCreationRequests for each of the artifacts w/o hash computation.
// It returns an error, if
// - any of the artifact IDs or contentTypes are invalid,
// - the total size exceeds MaxBatchCreateArtifactSize, or
// - there are more than one invocations associated with the artifacts.
// - both data and a GCS URI are supplied
func parseBatchCreateArtifactsRequest(in *pb.BatchCreateArtifactsRequest) (invocations.ID, []*artifactCreationRequest, error) {
	var tSize int64
	var invID invocations.ID

	if err := pbutil.ValidateBatchRequestCount(len(in.Requests)); err != nil {
		return "", nil, err
	}
	arts := make([]*artifactCreationRequest, len(in.Requests))
	for i, req := range in.Requests {
		inv, art, err := parseCreateArtifactRequest(req)
		if err != nil {
			return "", nil, errors.Annotate(err, "requests[%d]", i).Err()
		}
		switch {
		case invID == "":
			invID = inv
		case invID != inv:
			return "", nil, errors.Reason("requests[%d]: only one invocation is allowed: %q, %q", i, invID, inv).Err()
		}

		// TODO(ddoman): limit the max request body size in prpc level.
		tSize += art.size
		if tSize > MaxBatchCreateArtifactSize {
			return "", nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchCreateArtifactSize).Err()
		}
		arts[i] = art
	}
	return invID, arts, nil
}

// findNewArtifacts returns a list of the artifacts that don't have states yet.
// If one exists w/ different hash/size, this returns an error.
func findNewArtifacts(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	// artifacts are not expected to exist in most cases, and this map would likely
	// be empty.
	type state struct {
		hash   string
		size   int64
		gcsURI string
	}
	var states map[string]state
	ks := spanner.KeySets()
	for _, a := range arts {
		ks = spanner.KeySets(invID.Key(a.parentID(), a.artifactID), ks)
	}
	var b spanutil.Buffer
	err := span.Read(ctx, "Artifacts", ks, []string{"ParentId", "ArtifactId", "RBECASHash", "Size", "GcsURI"}).Do(
		func(row *spanner.Row) (err error) {
			var pid, aid string
			var hash string
			var size = new(int64)
			var gcsURI string
			if err = b.FromSpanner(row, &pid, &aid, &hash, &size, &gcsURI); err != nil {
				return
			}
			if states == nil {
				states = make(map[string]state)
			}
			// treat non-existing size as 0.
			if size == nil {
				size = new(int64)
			}
			// The artifact exists.
			states[invID.Key(pid, aid).String()] = state{hash, *size, gcsURI}
			return
		},
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "%s", err)
	}

	newArts := make([]*artifactCreationRequest, 0, len(arts)-len(states))
	for _, a := range arts {
		// Save the hash, so that it can be reused in the post-verification
		// after rbecase.UpdateBlob().
		if a.gcsURI == "" && a.hash == "" {
			h := sha256.Sum256(a.data)
			a.hash = artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
		}
		st, ok := states[invID.Key(a.parentID(), a.artifactID).String()]
		if !ok {
			newArts = append(newArts, a)
			continue
		}
		if (a.gcsURI == "") != (st.gcsURI == "") {
			// Can't change from GCS to non-GCS and vice-versa
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different storage scheme`, a.name(invID))
		}
		if a.size != st.size {
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different size: %d != %d`, a.name(invID), a.size, st.size)
		}
		if a.gcsURI != "" {
			if a.gcsURI != st.gcsURI {
				return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different GCS URI: %s != %s`, a.name(invID), a.gcsURI, st.gcsURI)
			}
		} else {
			if a.hash != st.hash {
				return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different hash`, a.name(invID))
			}
		}
	}
	return newArts, nil
}

// checkArtStates checks if the states of the associated invocation and artifacts are
// compatible with creation of the artifacts. On success, it returns a list of
// the artifactCreationRequests of which artifact don't have states in Spanner yet.
func checkArtStates(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) (reqs []*artifactCreationRequest, invInfo *invocationInfo, err error) {
	var invState pb.Invocation_State
	var createTime time.Time
	var realm string

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return invocations.ReadColumns(ctx, invID, map[string]any{
			"State": &invState, "Realm": &realm, "CreateTime": &createTime,
		})
	})

	eg.Go(func() (err error) {
		reqs, err = findNewArtifacts(ctx, invID, arts)
		return
	})

	switch err := eg.Wait(); {
	case err != nil:
		return nil, nil, err
	case invState != pb.Invocation_ACTIVE:
		return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "%s is not active", invID.Name())
	}
	return reqs, &invocationInfo{
		id:         string(invID),
		realm:      realm,
		createTime: createTime,
	}, nil
}

// createArtifactStates creates the states of given artifacts in Spanner.
func createArtifactStates(ctx context.Context, realm string, invID invocations.ID, arts []*artifactCreationRequest) error {
	var noStateArts []*artifactCreationRequest
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) (err error) {
		// Verify all the states again.
		noStateArts, _, err = checkArtStates(ctx, invID, arts)
		if err != nil {
			return err
		}
		if len(noStateArts) == 0 {
			logging.Warningf(ctx, "The states of all the artifacts already exist.")
		}
		for _, a := range noStateArts {
			span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]any{
				"InvocationId": invID,
				"ParentId":     a.parentID(),
				"ArtifactId":   a.artifactID,
				"ContentType":  a.contentType,
				"Size":         a.size,
				"RBECASHash":   a.hash,
				"GcsURI":       a.gcsURI,
			}))
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to write artifact to Spanner").Err()
	}
	spanutil.IncRowCount(ctx, len(noStateArts), spanutil.Artifacts, spanutil.Inserted, realm)
	return nil
}

func uploadArtifactBlobs(ctx context.Context, rbeIns string, casClient repb.ContentAddressableStorageClient, invID invocations.ID, arts []*artifactCreationRequest) error {
	casReq := &repb.BatchUpdateBlobsRequest{InstanceName: rbeIns}
	for _, a := range arts {
		casReq.Requests = append(casReq.Requests, &repb.BatchUpdateBlobsRequest_Request{
			Digest: &repb.Digest{Hash: artifacts.TrimHashPrefix(a.hash), SizeBytes: a.size},
			Data:   a.data,
		})
	}
	resp, err := casClient.BatchUpdateBlobs(ctx, casReq, &grpc.MaxSendMsgSizeCallOption{MaxSendMsgSize: MaxBatchCreateArtifactSize})
	if err != nil {
		// If BatchUpdateBlobs() returns INVALID_ARGUMENT, it means that
		// the total size of the artifact contents was bigger than the max size that
		// BatchUpdateBlobs() can accept.
		return errors.Annotate(err, "cas.BatchUpdateBlobs failed").Err()
	}
	for i, r := range resp.GetResponses() {
		cd := codes.Code(r.Status.Code)
		if cd != codes.OK {
			// Each individual error can be due to resource exhausted or unmatched digest.
			// If unmatched digest, this RPC has a bug and needs to be fixed.
			// If resource exhausted, the RBE server quota needs to be adjusted.
			//
			// Either case, it's a server-error, and an internal error will be returned.
			return errors.Reason("artifact %q: cas.BatchUpdateBlobs failed", arts[i].name(invID)).Err()
		}
	}
	return nil
}

// allowedBucketsForUser returns the GCS buckets a user is allowed to reference by reading
// the project config.
// If no config exists for the user, an empty map will be returned, rather than an error.
func allowedBucketsForUser(ctx context.Context, project, user string) (allowedBuckets map[string]bool, err error) {
	allowedBuckets = map[string]bool{}
	// This is cached for 1 minute, so no need to re-optimize here.
	cfg, err := config.Project(ctx, project)
	if err != nil {
		if errors.Is(err, config.ErrNotFoundProjectConfig) {
			return allowedBuckets, nil
		}
		return nil, err
	}

	for _, list := range cfg.GcsAllowList {
		for _, listUser := range list.Users {
			if listUser == user {
				for _, bucket := range list.Buckets {
					allowedBuckets[bucket] = true
				}
				return allowedBuckets, nil
			}
		}
	}
	return allowedBuckets, nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
// This functions uploads the artifacts to RBE-CAS.
// If the artifact is a text-based artifact, it will also get uploaded to BigQuery.
// We have a percentage control to determine how many percent of artifacts got
// uploaded to BigQuery.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.Requests) == 0 {
		logging.Debugf(ctx, "Received a BatchCreateArtifactsRequest with 0 requests; returning")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}
	invID, arts, err := parseBatchCreateArtifactsRequest(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	var artsToCreate []*artifactCreationRequest
	var invInfo *invocationInfo
	func() {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()
		artsToCreate, invInfo, err = checkArtStates(ctx, invID, arts)
	}()
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}
	realm := invInfo.realm
	project, _ := realms.Split(realm)
	user := auth.CurrentUser(ctx).Identity

	var allowedBuckets map[string]bool = nil
	artsToUpload := make([]*artifactCreationRequest, 0, len(artsToCreate))
	for _, a := range artsToCreate {
		// Only upload to RBE CAS the ones that are not in GCS
		if a.gcsURI == "" {
			artsToUpload = append(artsToUpload, a)
		} else {
			// Check this GCS reference is allowed by the project config.
			// Delay construction of the checker (which may occasionally involve an RPC) until we know we
			// actually need it.
			if allowedBuckets == nil {
				allowedBuckets, err = allowedBucketsForUser(ctx, project, string(user))
				if err != nil {
					return nil, errors.Annotate(err, "fetch allowed buckets for user %s", string(user)).Err()
				}
			}
			bucket, _ := gsutil.Split(a.gcsURI)
			if _, ok := allowedBuckets[bucket]; !ok {
				return nil, errors.New(fmt.Sprintf("the user %s does not have permission to reference GCS objects in bucket %s in project %s", string(user), bucket, project))
			}
		}
	}

	if err := uploadArtifactBlobs(ctx, s.ArtifactRBEInstance, s.casClient, invID, artsToUpload); err != nil {
		return nil, err
	}
	if err := createArtifactStates(ctx, realm, invID, artsToCreate); err != nil {
		return nil, err
	}

	// Upload text artifact to BQ.
	shouldUpload, err := shouldUploadToBQ(ctx)
	if err != nil {
		// Just log here, the feature is still in experiment, and we do not want
		// to disturb the main flow.
		err = errors.Annotate(err, "getting should upload to BQ").Err()
		logging.Errorf(ctx, err.Error())
	} else {
		if !shouldUpload {
			// Just disable the logging for now because the feature is disabled.
			// We will enable back when we enable the export.
			// logging.Infof(ctx, "Uploading artifacts to BQ is disabled")
		} else {
			err = processBQUpload(ctx, s.bqExportClient, artsToCreate, invInfo)
			if err != nil {
				// Just log here, the feature is still in experiment, and we do not want
				// to disturb the main flow.
				err = errors.Annotate(err, "processBQUpload").Err()
				logging.Errorf(ctx, err.Error())
			}
		}
	}

	// Return all the artifacts to indicate that they were created.
	ret := &pb.BatchCreateArtifactsResponse{Artifacts: make([]*pb.Artifact, len(arts))}
	for i, a := range arts {
		ret.Artifacts[i] = &pb.Artifact{
			Name:        a.name(invID),
			ArtifactId:  a.artifactID,
			ContentType: a.contentType,
			SizeBytes:   a.size,
		}
	}
	return ret, nil
}

// processBQUpload filters text artifacts and upload to BigQuery.
func processBQUpload(ctx context.Context, client BQExportClient, artifactRequests []*artifactCreationRequest, invInfo *invocationInfo) error {
	if client == nil {
		return errors.New("bq export client should not be nil")
	}
	textArtifactRequests := filterTextArtifactRequests(ctx, artifactRequests, invInfo)
	percent, err := percentOfArtifactsToBQ(ctx)
	if err != nil {
		return errors.Annotate(err, "getting percent of artifact to upload to BQ").Err()
	}
	textArtifactRequests, err = throttleArtifactsForBQ(textArtifactRequests, percent)
	if err != nil {
		return errors.Annotate(err, "throttle artifacts for bq").Err()
	} else {
		err = uploadArtifactsToBQ(ctx, client, textArtifactRequests, invInfo)
		if err != nil {
			return errors.Annotate(err, "uploadArtifactsToBQ").Err()
		}
	}
	return nil
}

// filterTextArtifactRequests filters only text artifacts.
func filterTextArtifactRequests(ctx context.Context, artifactRequests []*artifactCreationRequest, invInfo *invocationInfo) []*artifactCreationRequest {
	project, _ := realms.Split(invInfo.realm)
	results := []*artifactCreationRequest{}
	for _, req := range artifactRequests {
		if req.contentType == "" {
			artifactContentCounter.Add(ctx, 1, project, "empty")
		} else {
			if pbutil.IsTextArtifact(req.contentType) {
				results = append(results, req)
				artifactContentCounter.Add(ctx, 1, project, "text")
			} else {
				artifactContentCounter.Add(ctx, 1, project, "nontext")
			}
		}
	}
	return results
}

// throttleArtifactsForBQ limits the artifacts being to BigQuery based on percentage.
// It will allow us to roll out the feature slowly.
func throttleArtifactsForBQ(artifactRequests []*artifactCreationRequest, percent int) ([]*artifactCreationRequest, error) {
	results := []*artifactCreationRequest{}
	for _, req := range artifactRequests {
		hashStr := fmt.Sprintf("%s%s", req.testID, req.artifactID)
		hashVal := hash64([]byte(hashStr))
		if hashVal%100 < uint64(percent) {
			results = append(results, req)
		}
	}
	return results, nil
}

// hash64 returns a hash value (uint64) for a given string.
func hash64(bt []byte) uint64 {
	hasher := fnv.New64a()
	hasher.Write(bt)
	return hasher.Sum64()
}

// percentOfArtifactsToBQ returns how many percents of artifact to be uploaded.
// Return value is an integer between [0, 100].
func percentOfArtifactsToBQ(ctx context.Context) (int, error) {
	cfg, err := config.GetServiceConfig(ctx)
	if err != nil {
		return 0, errors.Annotate(err, "get service config").Err()
	}
	return int(cfg.GetBqArtifactExportConfig().GetExportPercent()), nil
}

// shouldUploadToBQ returns true if we should upload artifacts to BigQuery.
// Note: Although we can also disable upload by setting percentOfArtifactsToBQ = 0,
// but it will also run some BQ exporter code.
// Disable shouldUploadToBQ flag will run no exporter code, therefore it is the safer option.
func shouldUploadToBQ(ctx context.Context) (bool, error) {
	cfg, err := config.GetServiceConfig(ctx)
	if err != nil {
		return false, errors.Annotate(err, "get service config").Err()
	}
	return cfg.GetBqArtifactExportConfig().GetEnabled(), nil
}

func uploadArtifactsToBQ(ctx context.Context, client BQExportClient, reqs []*artifactCreationRequest, invInfo *invocationInfo) error {
	project, _ := realms.Split(invInfo.realm)
	rowsToUpload := []*bqpb.TextArtifactRow{}
	for _, req := range reqs {
		// Some artifacts uploaded contains invalid Unicode.
		// If it is the case, we will just skip those artifacts and log a warning.
		// We do not use logging.Error because it will make it harder
		// to review the log viewer for the logs we are concerned about.
		if !utf8.Valid(req.data) {
			logging.Warningf(ctx, "Invalid UTF-8 content. Inv ID: %s. Test ID: %s. Artifact ID: %s.", invInfo.id, req.testID, req.artifactID)
			artifactExportCounter.Add(ctx, 1, project, "failure_input")
			continue
		}
		rows, err := reqToProtos(ctx, req, invInfo, req.testStatus, MaxShardContentSize, LookbackWindow)
		if err != nil {
			return errors.Annotate(err, "req to protos").Err()
		}
		rowsToUpload = append(rowsToUpload, rows...)
	}
	logging.Infof(ctx, "Uploading %d rows BQ", len(rowsToUpload))
	if len(rowsToUpload) > 0 {
		err := client.InsertArtifactRows(ctx, rowsToUpload)
		if err != nil {
			// Data is invalid.
			if _, ok := errors.TagValueIn(bqutil.InvalidRowTagKey, err); ok {
				artifactExportCounter.Add(ctx, int64(len(rowsToUpload)), rowsToUpload[0].Project, "failure_input")
			} else {
				artifactExportCounter.Add(ctx, int64(len(rowsToUpload)), rowsToUpload[0].Project, "failure_bq")
			}
			return errors.Annotate(err, "insert artifact rows").Err()
		} else {
			artifactExportCounter.Add(ctx, int64(len(rowsToUpload)), rowsToUpload[0].Project, "success")
		}
	}
	return nil
}

func reqToProtos(ctx context.Context, req *artifactCreationRequest, invInfo *invocationInfo, status pb.TestStatus, maxSize int, lookbackWindow int) ([]*bqpb.TextArtifactRow, error) {
	chunks, err := util.SplitToChunks(req.data, maxSize, lookbackWindow)
	if err != nil {
		return nil, errors.Annotate(err, "split to chunk").Err()
	}
	results := []*bqpb.TextArtifactRow{}
	project, realm := realms.Split(invInfo.realm)
	for i, chunk := range chunks {
		results = append(results, &bqpb.TextArtifactRow{
			Project:             project,
			Realm:               realm,
			InvocationId:        invInfo.id,
			TestId:              req.testID,
			ResultId:            req.resultID,
			ArtifactId:          req.artifactID,
			ContentType:         req.contentType,
			NumShards:           int32(len(chunks)),
			ShardId:             int32(i),
			Content:             chunk,
			ShardContentSize:    int32(len(chunk)),
			ArtifactContentSize: int32(req.size),
			PartitionTime:       timestamppb.New(invInfo.createTime),
			ArtifactShard:       fmt.Sprintf("%s:%d", req.artifactID, i),
			TestStatus:          testStatusToString(status),
		})
	}
	return results, nil
}

func testStatusToString(status pb.TestStatus) string {
	if status == pb.TestStatus_STATUS_UNSPECIFIED {
		return ""
	}
	return pb.TestStatus_name[int32(status)]
}
