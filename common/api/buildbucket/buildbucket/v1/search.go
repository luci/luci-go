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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// Fetch fetches builds matching the search criteria.
// It stops when all builds are found or when context is cancelled.
//
// The order of returned builds is from most-recently-created to least-recently-created.
//
// If ret is nil, retries transient errors with exponential back-off.
// Logs errors on retries.
//
// Returns nil only if the search results are exhausted.
// May return context.Canceled.
func (c *SearchCall) Fetch(ret retry.Factory) ([]*ApiCommonBuildMessage, error) {
	ch := make(chan *ApiCommonBuildMessage)
	var err error
	go func() {
		defer close(ch)
		err = c.Run(ch, ret)
	}()

	var builds []*ApiCommonBuildMessage
	for b := range ch {
		builds = append(builds, b)
	}
	return builds, err
}

// Run is like Fetch, but sends results to the builds channel and
// blocks no sending.
// The context may be canceled between sending two builds of the same result
// page.
func (c *SearchCall) Run(builds chan<- *ApiCommonBuildMessage, ret retry.Factory) error {
	if ret == nil {
		ret = transient.Only(retry.Default)
	}

	// We will be mutating c.
	// Guarantee that it will remain the same by the time function exits.
	origCtx := c.ctx_
	const cursorKey = "start_cursor"
	origCursor := c.urlParams_.Get(cursorKey)
	defer func() {
		// these calls are equivalent to c.Context(origCtx) and
		// c.StartCursor(origCursor). Use the low-level API to
		// be consistent with reads.
		c.ctx_ = origCtx
		c.urlParams_.Set(cursorKey, origCursor)
	}()

	// Make a non-nil context used by default in this function.
	ctx := origCtx
	if ctx == nil {
		// won't happen on AppEngine in practice.
		ctx = context.Background()
	}

	for {
		var res *ApiSearchResponseMessage
		err := retry.Retry(ctx, ret,
			func() error {
				var err error
				// Set a timeout for this particular RPC.
				c.ctx_, _ = context.WithTimeout(ctx, time.Minute)
				res, err = c.Do()
				c.ctx_ = origCtx // for code clarity only

				switch apiErr, _ := err.(*googleapi.Error); {
				case apiErr != nil && apiErr.Code >= 500:
					return transient.Tag.Apply(err)
				case err == context.DeadlineExceeded && ctx.Err() == nil:
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
				logging.WithError(err).Warningf(ctx, "RPC error while searching builds; will retry in %s", wait)
			})
		if err != nil {
			return err
		}

		for _, b := range res.Builds {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case builds <- b:
			}
		}

		if len(res.Builds) == 0 || res.NextCursor == "" {
			break
		}
		c.urlParams_.Set(cursorKey, res.NextCursor)
	}

	return nil
}
