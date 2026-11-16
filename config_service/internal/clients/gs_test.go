// Copyright 2026 The LUCI Authors.
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

package clients_test

import (
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	gomock "github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/testutil"
)

func TestCreateSignedURLs(t *testing.T) {
	t.Parallel()

	ctx := testutil.SetupContext()
	ctl := gomock.NewController(t)
	mockGsClient := clients.NewMockGsClient(ctl)
	paths := []gs.Path{
		gs.MakePath("test-bucket-1", "test-object-1"),
		gs.MakePath("test-bucket-2", "test-object-2"),
	}

	var recordedOpts *storage.SignedURLOptions
	mockGsClient.EXPECT().SignedURL(
		gomock.Eq("test-bucket-1"),
		gomock.Eq("test-object-1"),
		gomock.AssignableToTypeOf(recordedOpts),
	).DoAndReturn(
		func(_, _ string, opts *storage.SignedURLOptions) (string, error) {
			recordedOpts = opts
			return "signed-url-1", nil
		},
	)
	mockGsClient.EXPECT().SignedURL(
		gomock.Eq("test-bucket-2"),
		gomock.Eq("test-object-2"),
		gomock.Any(),
	).Return("signed-url-2", nil)

	urls, err := clients.CreateSignedURLs(ctx, mockGsClient, paths, http.MethodGet, map[string]string{
		"Custom-Header":         "Value",
		"Another-Custom-Header": "Another-Value",
	})
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, urls, should.Match([]string{"signed-url-1", "signed-url-2"}))
	assert.Loosely(t, recordedOpts.GoogleAccessID, should.Equal(testutil.ServiceAccount))
	assert.Loosely(t, recordedOpts.Scheme, should.Equal(storage.SigningSchemeV4))
	assert.Loosely(t, recordedOpts.Method, should.Equal(http.MethodGet))
	assert.Loosely(t, recordedOpts.Expires, should.Match(clock.Now(ctx).Add(clients.SignedURLExpireDur)))
	assert.Loosely(t, recordedOpts.Headers, should.Match([]string{"Another-Custom-Header:Another-Value", "Custom-Header:Value"}))
}
