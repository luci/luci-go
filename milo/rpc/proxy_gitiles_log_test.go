// Copyright 2024 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	milopb "go.chromium.org/luci/milo/proto/v1"
)

func TestValidateProxyGitilesLogRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateGitilesLogRequest`, t, func(t *ftt.Test) {
		t.Run(`reject invalid host`, func(t *ftt.Test) {
			err := validateProxyGitilesLogRequest(&milopb.ProxyGitilesLogRequest{
				Host:    "invalid.host",
				Request: &gitiles.LogRequest{},
			})
			assert.Loosely(t, err, should.ErrLike("host must be a subdomain of .googlesource.com"))
		})

		t.Run(`valid`, func(t *ftt.Test) {
			err := validateProxyGitilesLogRequest(&milopb.ProxyGitilesLogRequest{
				Host:    "chromium.googlesource.com",
				Request: &gitiles.LogRequest{},
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
