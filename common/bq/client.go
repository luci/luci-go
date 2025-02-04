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

package bq

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// NewClient returns a new BigQuery client for use with the given GCP project,
// that authenticates as the LUCI service itself.
func NewClient(ctx context.Context, gcpProject string) (*bigquery.Client, error) {
	if gcpProject == "" {
		return nil, errors.New("GCP Project must be specified")
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(bigquery.Scope))
	if err != nil {
		return nil, err
	}
	return bigquery.NewClient(ctx, gcpProject, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
}
