// Copyright 2018 The LUCI Authors.
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

package processing

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/cipd/appengine/impl/model"
)

// Processor runs some post-processing step on a package instance after it has
// been uploaded.
type Processor interface {
	// ID is unique identifier of this processor used store processing results in
	// the datastore.
	ID() string

	// Applicable returns true if this processor should be applied to an instance.
	Applicable(inst *model.Instance) bool

	// Run executes the processing on the package instance.
	//
	// Returns either a result, or a transient error. All fatal errors should be
	// communicated through the result.
	//
	// Must be idempotent. The processor may be called multiple times when
	// retrying task queue tasks.
	Run(ctx context.Context, inst *model.Instance, pkg *PackageReader) (Result, error)
}

// Result is returned by processors.
//
// It is either some JSON-serializable data or a fatal error.
type Result struct {
	Result interface{} // JSON-serializable summary extracted by the processor
	Err    error       // if non-nil, contains the fatal error message
}
