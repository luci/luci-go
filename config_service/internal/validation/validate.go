// Copyright 2023 The LUCI Authors.
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

package validation

import (
	"context"

	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
)

// finder defines the interface for validator to find the corresponding services
// to validate the given config file.
type finder interface {
	FindInterestedServices(cs config.Set, filePath string) []*model.Service
}

// Validator validates config files backed Google Cloud Storage.
type Validator struct {
	// GsClient is used to talk Google Cloud Storage
	GsClient clients.GsClient
	// Finder is used to find the services that can validate a given config files.
	Finder finder
}

// File defines the interface of a config file in validation.
type File interface {
	// GetPath returns the relative path to the config file from config root.
	GetPath() string
	// GetGSPath returns the GCS path to where the config file is stored.
	//
	// The GCS object that the path points to SHOULD exist before calling
	// `Validate`.
	GetGSPath() gs.Path
	// TODO(yiwzhang): add GetRawContent to support legacy validation protocol.
}

// ExamineResult is the result of `Examine` method.
type ExamineResult struct {
	// MissingFiles are files whose the corresponding GCS object doesn't exist.
	//
	// Use the attached Signed URL instruct client to upload the config content.
	MissingFiles []struct {
		File      File
		SignedURL string
	}
	// UnvalidatableFiles are files that no service can validate.
	//
	// Those files SHOULD not be included in the validation request.
	UnvalidatableFiles []File
}

// Passed return True if the config files passed the examination and can
// proceed to `Validate`.
func (er *ExamineResult) Passed() bool {
	return er == nil || (len(er.MissingFiles) == 0 && len(er.UnvalidatableFiles) == 0)
}

// Examine examines the configs files to ensure successful validation.
func (v *Validator) Examine(ctx context.Context, cs config.Set, files []File) (*ExamineResult, error) {
	panic("unimplemented")
}

// Validate validates the provided config files.
func (v *Validator) Validate(ctx context.Context, cs config.Set, files []File) (*cfgcommonpb.ValidationResult, error) {
	panic("unimplemented")
}
