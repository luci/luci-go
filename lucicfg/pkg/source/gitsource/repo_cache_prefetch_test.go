// Copyright 2025 The LUCI Authors.
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

package gitsource

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/lucicfg/pkg/source"
)

func TestPrefetchMultiple(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	// bunch of objects from commit 9c350ff8eeacef414a4baa5d68098e61b2d51a51
	objects := []string{
		"6371c00ed96239f955ecdb11c96fcd578eff821d",
		"637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96",
		"fa70b1cd267f69a7a6ea237364ae3b71907b5005",
		"0a2bb89ae387fbbfe5e2eecf5a1c49f45e30e5dc",
		"9acec8c9a4931845dea5669205e319498c787844",
		"05531497834273d365cc3f1c318069ed0e222b9d",
		"3573c9a2ac92f0f96a5e53fa29c10e5fd09c983b",
		"b423929a035c9209e85661588be48f304585b0bb",
		"37f1199eb96be259092fb9906d4d412ea7357f2d",
		"366cd2cbff6025c2ace384a7e43a5b81f29ebc75",
	}

	// make sure that we don't already have these objects in our cache repo
	for _, obj := range objects {
		_, _, err := repo.batchProc.catFile(context.Background(), obj)
		assert.ErrIsLike(t, err, source.ErrMissingObject)
	}

	// prefetch them all
	err := repo.prefetchMultiple(context.Background(), objects)
	assert.NoErr(t, err)

	// now they are all present (note that we didn't have to restart our
	// git cat-file background process).
	for _, obj := range objects {
		_, _, err := repo.batchProc.catFile(context.Background(), obj)
		assert.NoErr(t, err)
	}
}
