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
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
)

// finder defines the interface for validator to find the corresponding services
// to validate the given config file.
type finder interface {
	FindInterestedServices(ctx context.Context, cs config.Set, filePath string) []*model.Service
}

// Validator validates config files backed Google Cloud Storage.
type Validator struct {
	// GsClient is used to talk Google Cloud Storage
	GsClient clients.GsClient
	// Finder is used to find the services that can validate a given config files.
	Finder finder
	// SelfRuleSet is the RuleSet that validates the configs against LUCI Config
	// itself.
	SelfRuleSet *validation.RuleSet
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
	// GetRawContent returns the raw and uncompressed content of this config.
	//
	// This is currently used to validate the configs LUCI Config itself is
	// interested in and validate configs against legacy services.
	GetRawContent(context.Context) ([]byte, error)
}

// ensuring *model.File implements File interface
var _ File = (*model.File)(nil)

// Validate validates the provided config files.
func (v *Validator) Validate(ctx context.Context, cs config.Set, files []File) (*cfgcommonpb.ValidationResult, error) {
	srvValidators := v.makeServiceValidators(ctx, cs, files)
	if len(srvValidators) == 0 { // no service can validate input files.
		return &cfgcommonpb.ValidationResult{}, nil
	}
	results := make([]*cfgcommonpb.ValidationResult, len(srvValidators))
	eg, ectx := errgroup.WithContext(ctx)
	for i, sv := range srvValidators {
		eg.Go(func() (err error) {
			filePaths := make([]string, len(sv.files))
			for i, file := range sv.files {
				filePaths[i] = file.GetPath()
			}
			logging.Debugf(ctx, "sending files [%s] to service %q to validate", filePaths, sv.service.Name)
			switch results[i], err = sv.validate(ectx); {
			case errors.Is(err, context.Canceled):
				logging.Warningf(ctx, "validating configs against service %q for files [%s] is cancelled", sv.service.Name, filePaths)
				return err
			case err != nil:
				err = fmt.Errorf("failed to validate configs against service %q for files [%s]: %w", sv.service.Name, filePaths, err)
				logging.Errorf(ctx, "%s", err)
				return err
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return mergeResults(results), nil
}

// makeServiceValidators constructs `serviceValidator`s with files each service
// is interested in. The `serviceValidator` will be used to validate the config
// later on.
func (v *Validator) makeServiceValidators(ctx context.Context, cs config.Set, files []File) []*serviceValidator {
	svs := make(map[string]*serviceValidator, len(files))
	for _, file := range files {
		services := v.Finder.FindInterestedServices(ctx, cs, file.GetPath())
		for _, service := range services {
			if _, ok := svs[service.Name]; !ok {
				svs[service.Name] = &serviceValidator{
					service:     service,
					gsClient:    v.GsClient,
					selfRuleSet: v.SelfRuleSet,
					cs:          cs,
				}
			}
			svs[service.Name].files = append(svs[service.Name].files, file)
		}
	}
	ret := make([]*serviceValidator, 0, len(svs))
	for _, v := range svs {
		ret = append(ret, v)
	}
	return ret
}

func mergeResults(results []*cfgcommonpb.ValidationResult) *cfgcommonpb.ValidationResult {
	ret := &cfgcommonpb.ValidationResult{}
	for _, res := range results {
		ret.Messages = append(ret.Messages, res.GetMessages()...)
	}
	// Sort lexicographically by path first and then sort from most severe to
	// least severe if path is the same.
	sort.SliceStable(ret.Messages, func(i, j int) bool {
		switch res := strings.Compare(ret.Messages[i].GetPath(), ret.Messages[j].GetPath()); res {
		case 0:
			return ret.Messages[i].GetSeverity() > ret.Messages[j].GetSeverity()
		default:
			return res < 0
		}
	})
	return ret
}
