// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
)

// Precision is result of algorithm precision evaluation.
// A precise algorithm selects only affected tests.
type Precision struct {
	// TODO(crbug.com/1112125): add fields.
}

func (r *evalRun) evaluatePrecision(ctx context.Context) (*Precision, error) {
	// TODO(crbug.com/1112125): implement it.
	return &Precision{}, nil
}
