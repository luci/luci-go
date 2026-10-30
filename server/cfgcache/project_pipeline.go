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
	"fmt"

	"google.golang.org/protobuf/proto"
)

// ProjectPipeline describes a pipeline to consume a snapshot of one or more
// config files across all LUCI projects (one snapshot per project).
//
// This pipeline will validate each project's snapshot and save it as
// `StoreMsg`.
//
// This specifically uses the config set pattern of `regex:projects/.+`.
type ProjectPipeline[StoreMsg proto.Message] struct {
	// ID uniquely identifies this pipeline within a [Registry] and in storage.
	//
	// Required.
	ID string

	// Store is the required storage for storing the transformed configuration.
	//
	// See 'dsstore' for a datastore-backed implementation (or 'memstore' for
	// testing).
	Store Store[StoreMsg]

	// Files is one or more [*SingleFile] or [*RegexFile] entries to consume
	// per-project.
	//
	// The Transform function will be invoked with a consistent [Snapshot] of all
	// of these for a project when any of them change.
	//
	// Required.
	Files []File

	// TransformVersion is the version of the Transform function's logic.
	//
	// This is stored alongside the ContentHash in storage so bumping
	// TransformVersion forces re-transformation and re-ingestion across
	// processes.
	//
	// This should be incremented any time the implementation of Transform is
	// updated.
	//
	// This may be unset (0) if Files consists of a single [*SingleFile] whose
	// type is StoreMsg. Otherwise this is required to be positive.
	TransformVersion int

	// Transform validates the parsed [Snapshot] and transforms it into the
	// StoreMsg proto to be cached in storage.
	//
	// Validation problems should be reported via c.Errorf(...) on the
	// validation.Context.
	//
	// Before Transform is called, c.SetFile is initialized to the resolved path
	// of the first [*SingleFile] in Files (if any). Remember to use c.SetFile to
	// attribute to other files as needed.
	//
	// If nil and Files consists of a single [*SingleFile] whose type is
	// StoreMsg, the parsed message is passed through unchanged. Otherwise this
	// is Required.
	Transform TransformFunc[StoreMsg]
}

func (pp *ProjectPipeline[StoreMsg]) String() string {
	return fmt.Sprintf("ProjectPipeline{ID: %q}", pp.ID)
}

// Register registers this pipeline with the given registry.
//
// Errors are buffered until [Registry.Initialize] is called. Panics if r is
// nil or already initialized. If using the Registry with the server Module,
// this must be called before server.Main, because the registered config
// implies module Dependencies.
//
// Registering enables the pipeline when the Registry is used with the server:
//   - Validation handlers will call TransformFunc.
func (pp *ProjectPipeline[StoreMsg]) Register(r *Registry) {
	register(r, pp.Files, pp.Transform, pp.Store, &ingestConfig{
		id:               pp.ID,
		desc:             pp.String(),
		transformVersion: pp.TransformVersion,
	})
}
