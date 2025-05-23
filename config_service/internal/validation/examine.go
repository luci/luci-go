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
	"net/http"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/config_service/internal/common"
)

// ExamineResult is the result of `Examine` method.
type ExamineResult struct {
	// MissingFiles are files that don't have the corresponding GCS objects.
	//
	// Use the attached signed URL to instruct client to upload the config
	// content.
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

var signedPutHeaders = map[string]string{
	"Content-Encoding":            "gzip",
	"x-goog-content-length-range": fmt.Sprintf("0,%d", common.ConfigMaxSize),
}

// Examine examines the configs files to ensure successful validation.
func (v *Validator) Examine(ctx context.Context, cs config.Set, files []File) (*ExamineResult, error) {
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	ret := &ExamineResult{}
	var mu sync.Mutex
	for _, file := range files {
		eg.Go(func() error {
			services := v.Finder.FindInterestedServices(ectx, cs, file.GetPath())
			if len(services) == 0 {
				mu.Lock()
				ret.UnvalidatableFiles = append(ret.UnvalidatableFiles, file)
				mu.Unlock()
				return nil
			}

			bucket, object := file.GetGSPath().Split()
			switch err := v.GsClient.Touch(ectx, bucket, object); {
			case errors.Is(err, context.Canceled):
				logging.Warningf(ctx, "touching config file %q is cancelled", file.GetPath())
				return err
			case errors.Is(err, storage.ErrObjectNotExist):
				urls, err := common.CreateSignedURLs(ectx, v.GsClient, []gs.Path{file.GetGSPath()}, http.MethodPut, signedPutHeaders)
				switch {
				case errors.Is(err, context.Canceled):
					logging.Warningf(ctx, "creating signed url for GS path %q is cancelled", file.GetGSPath())
					return err
				case err != nil:
					logging.Errorf(ctx, "failed to create signed url for GS path %q: %s", file.GetGSPath(), err)
					return err
				}
				mu.Lock()
				ret.MissingFiles = append(ret.MissingFiles, struct {
					File      File
					SignedURL string
				}{File: file, SignedURL: urls[0]})
				mu.Unlock()
				return nil
			case err != nil:
				logging.Errorf(ctx, "failed to touch config file %q: %s", file.GetPath(), err)
				return err
			default:
				return nil // file ready for validation.
			}
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	sort.SliceStable(ret.MissingFiles, func(i, j int) bool {
		return strings.Compare(ret.MissingFiles[i].File.GetPath(), ret.MissingFiles[j].File.GetPath()) < 0
	})
	sort.SliceStable(ret.UnvalidatableFiles, func(i, j int) bool {
		return strings.Compare(ret.UnvalidatableFiles[i].GetPath(), ret.UnvalidatableFiles[j].GetPath()) < 0
	})
	return ret, nil
}
