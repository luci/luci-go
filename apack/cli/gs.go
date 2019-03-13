// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"context"
	"net/http"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

func logWriting(ctx context.Context, o *storage.ObjectHandle) {
	logging.Infof(ctx, "writing gs://%s/%s\n", o.BucketName(), o.ObjectName())
}

func isGoogleAPINotFound(err error) bool {
	gerr, ok := err.(*googleapi.Error)
	return ok && gerr.Code == http.StatusNotFound
}

func newStorageClient(ctx context.Context, authOpts auth.Options) (*storage.Client, error) {
	httpClient, err := auth.NewAuthenticator(ctx, auth.InteractiveLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}

	return storage.NewClient(ctx, option.WithHTTPClient(httpClient))
}
