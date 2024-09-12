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

package recipespy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func BenchmarkBundlingBuildRepo(b *testing.B) {
	tmpDir := b.TempDir()
	checkout := filepath.Join(tmpDir, "checkout")
	_, err := git.PlainClone(checkout, false, &git.CloneOptions{
		URL:           "https://chromium.googlesource.com/chromium/tools/build",
		ReferenceName: plumbing.ReferenceName("refs/heads/main"),
		SingleBranch:  true,
		Depth:         1,
	})
	assert.That(b, err, should.ErrLike(nil))

	// this will download all the dependencies.
	cmd := exec.Command(filepath.Join("recipes", "recipes.py"), "fetch")
	cmd.Dir = checkout
	assert.That(b, cmd.Run(), should.ErrLike(nil))
	b.ResetTimer()
	for range b.N {
		bundleDir, err := os.MkdirTemp(tmpDir, "bundle_*")
		assert.That(b, err, should.ErrLike(nil))
		err = Bundle(context.Background(), checkout, bundleDir, nil)
		assert.That(b, err, should.ErrLike(nil))
	}
}
