// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSSHPluginMain(t *testing.T) {
	t.Parallel()

	t.Run("default args", func(t *testing.T) {
		got, err := parseArgs([]string{"helper"})
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(runArgs{
			Mode:    modePassthrough,
			Verbose: false,
			SSHArgs: []string{},
		}))
	})

	t.Run("without flags", func(t *testing.T) {
		got, err := parseArgs([]string{
			"helper", "who@host",
		})
		assert.NoErr(t, err)
		assert.That(t, got.SSHArgs, should.Match([]string{"who@host"}))
	})

	t.Run("mixed flags", func(t *testing.T) {
		got, err := parseArgs([]string{
			"helper", "--verbose", "who@host", "-p2222",
		})
		assert.NoErr(t, err)
		assert.That(t, got.Verbose, should.BeTrue)
		assert.That(t, got.SSHArgs, should.Match([]string{"who@host", "-p2222"}))
	})

	t.Run("mixed flags with delimiter", func(t *testing.T) {
		got, err := parseArgs([]string{
			"helper", "--verbose", "--", "-p2222", "who@host"})
		assert.NoErr(t, err)
		assert.That(t, got.Verbose, should.BeTrue)
		assert.That(t, got.SSHArgs, should.Match([]string{"-p2222", "who@host"}))
	})

	t.Run("mixed flags without delimiter", func(t *testing.T) {
		_, err := parseArgs([]string{
			"helper", "--verbose", "-p2222", "who@host"})
		assert.ErrIsLike(t, err, "flag provided but not defined")
	})
}
