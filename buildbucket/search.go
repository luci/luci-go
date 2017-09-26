// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// Search searches for builds and sends them to the builds channel
// until the context is cancelled or there are no more builds.
// Search may fetch multiple pages of results.
// The order of builds is from most-recently-created to least-recently-created.
//
// If ret is nil, retries transient errors with exponential back-off.
// Logs errors on retries.
//
// Returns nil only if the search results are exhausted.
func Search(c context.Context, req *buildbucket.SearchCall, builds chan<- *buildbucket.ApiCommonBuildMessage, ret retry.Factory) error {
	if ret == nil {
		ret = transient.Only(retry.Default)
	}
	for {
		var res *buildbucket.ApiSearchResponseMessage
		err := retry.Retry(c, ret,
			func() error {
				reqCtx, _ := context.WithTimeout(c, time.Minute)
				var err error
				res, err = req.Context(reqCtx).Do()
				switch apiErr, _ := err.(*googleapi.Error); {
				case apiErr != nil && apiErr.Code >= 500:
					return transient.Tag.Apply(err)
				case err == context.DeadlineExceeded && c.Err() == nil:
					return transient.Tag.Apply(err) // request-level timeout
				case err != nil:
					return err
				case res.Error != nil:
					return errors.New(res.Error.Message)
				default:
					return nil
				}
			},
			func(err error, wait time.Duration) {
				logging.WithError(err).Warningf(c, "RPC error while searching builds; will retry in %s", wait)
			})
		if err != nil {
			return err
		}

		for _, b := range res.Builds {
			select {
			case <-c.Done():
				return c.Err()
			case builds <- b:
			}
		}

		if len(res.Builds) == 0 || res.NextCursor == "" {
			break
		}
		req.StartCursor(res.NextCursor)
	}

	return nil
}

// SearchAll is similar to Search, but returns builds as a slice.
func SearchAll(c context.Context, req *buildbucket.SearchCall, ret retry.Factory) ([]*buildbucket.ApiCommonBuildMessage, error) {
	ch := make(chan *buildbucket.ApiCommonBuildMessage)
	var err error
	go func() {
		defer close(ch)
		err = Search(c, req, ch, ret)
	}()

	var builds []*buildbucket.ApiCommonBuildMessage
	for b := range ch {
		builds = append(builds, b)
	}
	return builds, err
}
