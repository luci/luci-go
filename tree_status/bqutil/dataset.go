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

// Package bqutil provides utility functions to interact with BigQuery.
package bqutil

import (
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/tree_status/pbutil"
)

// InternalDatasetID is the name of the BigQuery dataset which is intended
// for internal service use only.
const InternalDatasetID = "internal"

// TreeNameForDataset returns the tree name that corresponds
// to the given BigQuery dataset.
func TreeNameForDataset(dataset string) (string, error) {
	treeName := strings.ReplaceAll(dataset, "_", "-")
	if err := pbutil.ValidateTreeID(treeName); err != nil {
		return "", errors.Fmt("validate tree name: %w", err)
	}
	return treeName, nil
}
