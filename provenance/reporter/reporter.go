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
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	snooperpb "go.chromium.org/luci/provenance/api/snooperpb/v1"
)

// Report implements all provenance interfaces.
//
// This can be used as a caching opportunity by users for storing client for
// longer use.
type Report struct {
	sClient snooperpb.SelfReportClient
}

// ReportCipdAdmission reports a local cipd admission to provenance.
func (r *Report) ReportCipdAdmission(ctx context.Context, pkgName, iid string) error {
	req := &snooperpb.ReportCipdRequest{
		CipdReport: &snooperpb.CipdReport{
			PackageName: pkgName,
			Iid:         iid,
			EventTs:     timestamppb.New(clock.Now(ctx)),
		},
	}

	_, err := r.sClient.ReportCipd(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "failed to report cipd admission: %v", err)
		return err
	}
	return nil
}

// ReportGitCheckout reports a local git checkout/fetch to provenance.
func (r *Report) ReportGitCheckout(ctx context.Context, repo, commit, ref string) error {
	req := &snooperpb.ReportGitRequest{
		GitReport: &snooperpb.GitReport{
			Repo:    repo,
			Commit:  commit,
			Refs:    ref,
			EventTs: timestamppb.New(clock.Now(ctx)),
		},
	}

	_, err := r.sClient.ReportGit(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "failed to report git checkout: %v", err)
		return err
	}
	return nil
}

// ReportStage reports task stage via provenance local server.
func (r *Report) ReportStage(ctx context.Context, stage snooperpb.TaskStage, recipe string) error {
	// Must pass recipe name when reporting task start.
	if stage == snooperpb.TaskStage_STARTED && recipe == "" {
		logging.Errorf(ctx, "failed to export task stage")
		return fmt.Errorf("need to report recipe when task starts")
	}

	req := &snooperpb.ReportTaskStageRequest{
		TaskStage: stage,
		Timestamp: timestamppb.New(clock.Now(ctx)),
		// required when task starts
		Recipe: recipe,
	}

	_, err := r.sClient.ReportTaskStage(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "failed to export task stage: %v", err)
		return err
	}
	return nil
}

// ReportCipdDigest reports digest of built cipd package to provenance.
func (r *Report) ReportCipdDigest(ctx context.Context, digest, pkgName, iid string) error {
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
	if err != nil {
		logging.Errorf(ctx, "failed to export cipd artifact digest: %v", err)
		return err
	}
	return nil
}

// ReportGcsDigest reports digest of a built gcs app to provenance.
func (r *Report) ReportGcsDigest(ctx context.Context, digest, gcsURI string) error {
	req := &snooperpb.ReportArtifactDigestRequest{
		Digest: digest,
		Artifact: &snooperpb.Artifact{
			Kind: &snooperpb.Artifact_Gcs{
				Gcs: gcsURI,
			},
		},
	}

	_, err := r.sClient.ReportArtifactDigest(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "failed to export gcs artifact digest to spike: %v", err)
		return err
	}
	return nil
}
