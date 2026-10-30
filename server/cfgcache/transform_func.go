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

package cfgcache

import (
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config/validation"
)

type (
	// TransformFunc is the type of function used to validate and transform a
	// decoded [Snapshot] of configuration files from LUCI Config in order to
	// ingest them into the service.
	//
	// Before TransformFunc is invoked, c.SetFile is set to either:
	//   * The Path of the first [*SingleFile] in the pipeline; OR
	//   * "" if no [*SingleFile] is present.
	//
	// When validating a multi-file snapshot, TransformFunc should call c.SetFile
	// appropriately to indicate which file is the one generating any warnings or
	// errors.
	TransformFunc[StoreMsg proto.Message] func(c *validation.Context, snap *Snapshot) (StoreMsg, error)

	// genericTransformFunc is a type-erased TransformFunc dealing with raw proto
	// interfaces.
	genericTransformFunc = TransformFunc[proto.Message]
)
