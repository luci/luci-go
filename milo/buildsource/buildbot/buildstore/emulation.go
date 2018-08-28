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

package buildstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
)

var emulationEnabledKey = "emulation enabled"

// WithEmulation enables or disables emulation.
func WithEmulation(c context.Context, enabled bool) context.Context {
	return context.WithValue(c, &emulationEnabledKey, enabled)
}

// EmulationEnabled returns true if emulation is enabled in c.
func EmulationEnabled(c context.Context) bool {
	enabled, _ := c.Value(&emulationEnabledKey).(bool)
	return enabled
}

var bucketCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(128),
	GlobalNamespace: "bucket_of_master",
	Marshal: func(item interface{}) ([]byte, error) {
		return []byte(item.(string)), nil
	},
	Unmarshal: func(blob []byte) (interface{}, error) {
		return string(blob), nil
	},
}

// BucketOf returns LUCI bucket that the given buildbot master is migrating to.
// Returns "" if it is unknown.
func BucketOf(c context.Context, master string) (string, error) {
	v, err := bucketCache.GetOrCreate(c, master, func() (v interface{}, exp time.Duration, err error) {
		transport, err := auth.GetRPCTransport(c, auth.AsSelf)
		if err != nil {
			return nil, 0, err
		}
		u := fmt.Sprintf("https://luci-migration.appspot.com/masters/%s/?format=json", url.PathEscape(master))
		var bucket string
		err = retry.Retry(c, retry.Default, func() error {
			res, err := ctxhttp.Get(c, &http.Client{Transport: transport}, u)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			switch {
			case res.StatusCode == http.StatusNotFound:
				return nil
			case res.StatusCode != http.StatusOK:
				return errors.Reason("migration app returned HTTP %d", res.StatusCode).Err()
			}

			var body struct {
				Bucket string
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				return errors.Annotate(err, "could not decode migration app's response body").Err()
			}

			bucket = body.Bucket
			return nil
		}, retry.LogCallback(c, "luci-migration-get-master"))

		if err != nil {
			return nil, 0, err
		}
		// cache the result for 10min in process memory and memcache.
		return bucket, 10 * time.Minute, nil
	})

	bucket, _ := v.(string)
	return bucket, err
}
