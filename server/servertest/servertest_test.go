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

package servertest

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/authtest"
)

// TestEmptyServer is a smoke test that just tests starting an empty server.
func TestEmptyServer(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	srv, err := RunServer(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
}

// TestServerTestAndFakeAuthDb tests using a fake auth DB and servertest at the same time.
func TestServerTestAndFakeAuthDb(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	ctx := context.Background()

	testServer, err := RunServer(ctx, &Settings{
		Options: &server.Options{
			AuthDBProvider: (&authtest.FakeDB{}).AsProvider(),
		},
	})

	assert.That(t, err, should.ErrLike(nil))
	assert.Loosely(t, testServer, should.NotBeNil)
}
