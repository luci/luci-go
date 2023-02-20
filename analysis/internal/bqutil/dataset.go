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

// Package bqutil provides utility functions to interact with BigQuery.
package bqutil

import (
	"strings"

	"go.chromium.org/luci/analysis/internal/config"

	"go.chromium.org/luci/common/errors"
)

// DatasetForProject returns the name of the BigQuery dataset that contains
// the given project's data, in the LUCI Analysis GCP project.
func DatasetForProject(luciProject string) (string, error) {
	// The returned dataset may be used in SQL expressions, so we want to
	// be absolutely sure no SQL Injection is possible.
	if !config.ProjectRe.MatchString(luciProject) {
		return "", errors.New("invalid LUCI Project")
	}

	// The valid alphabet of LUCI project names [1] is [a-z0-9-] whereas
	// the valid alphabet of BQ dataset names [2] is [a-zA-Z0-9_].
	// [1]: https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/config/common.py?q=PROJECT_ID_PATTERN
	// [2]: https://cloud.google.com/bigquery/docs/datasets#dataset-naming
	return strings.ReplaceAll(luciProject, "-", "_"), nil
}

// ProjectForDataset returns the name of the LUCI Project that corresponds
// to the given BigQuery dataset.
func ProjectForDataset(dataset string) (string, error) {
	project := strings.ReplaceAll(dataset, "_", "-")

	if !config.ProjectRe.MatchString(project) {
		return "", errors.New("invalid LUCI Project")
	}

	return project, nil
}
