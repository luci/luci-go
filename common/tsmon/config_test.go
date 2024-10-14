// Copyright 2015 The LUCI Authors.
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

package tsmon

import (
	"io/ioutil"
	"os"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLoadConfig(t *testing.T) {
	ftt.Run("No file", t, func(t *ftt.Test) {
		c, err := loadConfig("")
		assert.Loosely(t, c.Credentials, should.BeEmpty)
		assert.Loosely(t, c.Endpoint, should.BeEmpty)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Missing file", t, func(t *ftt.Test) {
		c, err := loadConfig("/does/not/exist")
		assert.Loosely(t, c.Credentials, should.BeEmpty)
		assert.Loosely(t, c.Endpoint, should.BeEmpty)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Empty file", t, func(t *ftt.Test) {
		tf, err := ioutil.TempFile("", "config_test")
		if err != nil {
			t.Fail()
		}
		defer tf.Close()
		defer os.Remove(tf.Name())

		c, err := loadConfig(tf.Name())
		assert.Loosely(t, c.Endpoint, should.BeEmpty)
		assert.Loosely(t, c.Credentials, should.BeEmpty)
		assert.Loosely(t, c.AutoGenHostname, should.Equal(false))
		assert.Loosely(t, c.Hostname, should.BeEmpty)
		assert.Loosely(t, c.Region, should.BeEmpty)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Full file", t, func(t *ftt.Test) {
		tf, err := ioutil.TempFile("", "config_test")
		if err != nil {
			t.Fail()
		}
		defer tf.Close()
		defer os.Remove(tf.Name())

		tf.WriteString(`
			{"endpoint":         "foo",
			 "credentials":      "bar",
			 "autogen_hostname": true,
			 "hostname":         "test_host",
			 "region":           "test_region"
			}`)
		tf.Sync()

		c, err := loadConfig(tf.Name())
		assert.Loosely(t, c.Endpoint, should.Equal("foo"))
		assert.Loosely(t, c.Credentials, should.Equal("bar"))
		assert.Loosely(t, c.AutoGenHostname, should.Equal(true))
		assert.Loosely(t, c.Hostname, should.Equal("test_host"))
		assert.Loosely(t, c.Region, should.Equal("test_region"))
		assert.Loosely(t, err, should.BeNil)
	})
}
