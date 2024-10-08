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

package clients

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidatePubSubTopicName(t *testing.T) {
	ftt.Run("validateTopicName", t, func(t *ftt.Test) {
		t.Run("wrong topic name", func(t *ftt.Test) {
			_, _, err := ValidatePubSubTopicName("projects/adsf/")
			assert.Loosely(t, err, should.ErrLike(`topic "projects/adsf/" does not match "^projects/(.*)/topics/(.*)$"`))
		})
		t.Run("wrong project identifier", func(t *ftt.Test) {
			_, _, err := ValidatePubSubTopicName("projects/pro/topics/topic1")
			assert.Loosely(t, err, should.ErrLike(`cloud project id "pro" does not match "^[a-z]([a-z0-9-]){4,28}[a-z0-9]$"`))
		})
		t.Run("wrong topic id prefix", func(t *ftt.Test) {
			_, _, err := ValidatePubSubTopicName("projects/cloud-project/topics/goog11")
			assert.Loosely(t, err, should.ErrLike(`topic id "goog11" shouldn't begin with the string goog`))
		})
		t.Run("wrong topic id format", func(t *ftt.Test) {
			_, _, err := ValidatePubSubTopicName("projects/cloud-project/topics/abc##")
			assert.Loosely(t, err, should.ErrLike(`topic id "abc##" does not match "^[A-Za-z]([0-9A-Za-z\\._\\-~+%]){3,255}$"`))
		})
		t.Run("success", func(t *ftt.Test) {
			cloudProj, topic, err := ValidatePubSubTopicName("projects/cloud-project/topics/mytopic")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cloudProj, should.Equal("cloud-project"))
			assert.Loosely(t, topic, should.Equal("mytopic"))
		})
	})
}
