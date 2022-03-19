// Copyright 2022 The LUCI Authors.
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

package reporter

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	snooperpb "go.chromium.org/luci/provenance/api/snooperpb/v1"
)

var ErrServiceUnavailable = errors.New("local provenance service unavailable")

// Report implements all provenance interfaces.
//
// This can be used as a caching opportunity by users for storing client for
// longer use.
// TODO(crbug/1269830): Implement a custom retry logic for transient errors.
// Custom retry is needed because grpc status `Unavailable` is tagged as
// transient. In this application, `ErrServiceUnavailable` is a permanent but
// acceptable error code.
type Report struct {
	sClient snooperpb.SelfReportClient
}

// ReportCipdAdmission reports a local cipd admission to provenance.
//
// It returns a success status and annotated error. Status is to indicate user
// whether to block further execution.
// If local provenance service is unavailable, it will return an ok status and
// annotated error. This is to indicate, the user should continue normal
// execution.
// All other errors are annotated to indicate permanent failures.
func (r *Report) ReportCipdAdmission(ctx context.Context, pkgName, iid string) (bool, error) {
	req := &snooperpb.ReportCipdRequest{
		CipdReport: &snooperpb.CipdReport{
			PackageName: pkgName,
			Iid:         iid,
			EventTs:     timestamppb.New(clock.Now(ctx)),
		},
	}

	_, err := r.sClient.ReportCipd(ctx, req)
	switch errS, _ := status.FromError(err); errS.Code() {
	case codes.OK:
		logging.Infof(ctx, "success to report cipd admission")
		return true, nil
	case codes.Unavailable:
		logging.Errorf(ctx, "failed to report cipd admission: %v", ErrServiceUnavailable)
		return true, ErrServiceUnavailable
	default:
		logging.Errorf(ctx, "failed to report cipd admission: %v", err)
		return false, err
	}
}

// ReportGitCheckout reports a local git checkout/fetch to provenance.
//
// It returns a success status and annotated error. Status is to indicate user
// whether to block further execution.
// If local provenance service is unavailable, it will return an ok status and
// annotated error. This is to indicate, the user should continue normal
// execution.
// All other errors are annotated to indicate permanent failures.
func (r *Report) ReportGitCheckout(ctx context.Context, repo, commit, ref string) (bool, error) {
	req := &snooperpb.ReportGitRequest{
		GitReport: &snooperpb.GitReport{
			Repo:    repo,
			Commit:  commit,
			Refs:    ref,
			EventTs: timestamppb.New(clock.Now(ctx)),
		},
	}

	_, err := r.sClient.ReportGit(ctx, req)
	switch errS, _ := status.FromError(err); errS.Code() {
	case codes.OK:
		logging.Infof(ctx, "success to report git checkout")
		return true, nil
	case codes.Unavailable:
		logging.Errorf(ctx, "failed to report git checkout: %v", ErrServiceUnavailable)
		return true, ErrServiceUnavailable
	default:
		logging.Errorf(ctx, "failed to report git checkout: %v", err)
		return false, err
	}
}

// ReportStage reports task stage via provenance local server.
//
// It returns a success status and annotated error. Status is to indicate user
// whether to block further execution.
// If local provenance service is unavailable, it will return an ok status and
// annotated error. This is to indicate, the user should continue normal
// execution.
// All other errors are annotated to indicate permanent failures.
func (r *Report) ReportStage(ctx context.Context, stage snooperpb.TaskStage, recipe string) (bool, error) {
	// Must pass recipe name when reporting task start.
	if stage == snooperpb.TaskStage_STARTED && recipe == "" {
		logging.Errorf(ctx, "failed to export task stage")
		return false, fmt.Errorf("a recipe must be provided when task starts")
	}

	req := &snooperpb.ReportTaskStageRequest{
		TaskStage: stage,
		Timestamp: timestamppb.New(clock.Now(ctx)),
		// required when task starts
		Recipe: recipe,
	}

	_, err := r.sClient.ReportTaskStage(ctx, req)
	switch errS, _ := status.FromError(err); errS.Code() {
	case codes.OK:
		logging.Infof(ctx, "success to report task stage")
		return true, nil
	case codes.Unavailable:
		logging.Errorf(ctx, "failed to report task stage: %v", ErrServiceUnavailable)
		return true, ErrServiceUnavailable
	default:
		logging.Errorf(ctx, "failed to report task stage: %v", err)
		return false, err
	}
}

// ReportCipdDigest reports digest of built cipd package to provenance.
//
// It returns a success status and annotated error. Status is to indicate user
// whether to block further execution.
// If local provenance service is unavailable, it will return an ok status and
// annotated error. This is to indicate, the user should continue normal
// execution.
// All other errors are annotated to indicate permanent failures.
func (r *Report) ReportCipdDigest(ctx context.Context, digest, pkgName, iid string) (bool, error) {
	req := &snooperpb.ReportArtifactDigestRequest{
		Digest: digest,
		Artifact: &snooperpb.Artifact{
			Kind: &snooperpb.Artifact_Cipd{
				Cipd: &snooperpb.Artifact_CIPD{
					PackageName: pkgName,
					InstanceId:  iid,
				},
			},
		},
	}

	_, err := r.sClient.ReportArtifactDigest(ctx, req)
	switch errS, _ := status.FromError(err); errS.Code() {
	case codes.OK:
		logging.Infof(ctx, "success to report cipd digest")
		return true, nil
	case codes.Unavailable:
		logging.Errorf(ctx, "failed to report cipd digest: %v", ErrServiceUnavailable)
		return true, ErrServiceUnavailable
	default:
		logging.Errorf(ctx, "failed to report cipd digest: %v", err)
		return false, err
	}
}

// ReportGcsDigest reports digest of a built gcs app to provenance.
//
// It returns a success status and annotated error. Status is to indicate user
// whether to block further execution.
// If local provenance service is unavailable, it will return an ok status and
// annotated error. This is to indicate, the user should continue normal
// execution.
// All other errors are annotated to indicate permanent failures.
func (r *Report) ReportGcsDigest(ctx context.Context, digest, gcsURI string) (bool, error) {
	req := &snooperpb.ReportArtifactDigestRequest{
		Digest: digest,
		Artifact: &snooperpb.Artifact{
			Kind: &snooperpb.Artifact_Gcs{
				Gcs: gcsURI,
			},
		},
	}

	_, err := r.sClient.ReportArtifactDigest(ctx, req)
	switch errS, _ := status.FromError(err); errS.Code() {
	case codes.OK:
		logging.Infof(ctx, "success to report cipd digest")
		return true, nil
	case codes.Unavailable:
		logging.Errorf(ctx, "failed to report cipd digest: %v", ErrServiceUnavailable)
		return true, ErrServiceUnavailable
	default:
		logging.Errorf(ctx, "failed to report cipd digest: %v", err)
		return false, err
	}
}
