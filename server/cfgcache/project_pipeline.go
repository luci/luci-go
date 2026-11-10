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
	"context"
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
)

// PassthroughProjectPipeline is a shorthand for a [ProjectPipeline] whose
// stored and in-memory types are identical.
type PassthroughProjectPipeline[T proto.Message] = ProjectPipeline[T, T]

// ProjectPipeline describes a pipeline to consume a snapshot of one or more
// config files across all LUCI projects (one snapshot per project).
//
// This pipeline will validate each project's snapshot and save it as
// `StoreMsg`.
//
// This specifically uses the config set pattern of `regex:projects/.+`.
type ProjectPipeline[StoreMsg proto.Message, InMemory any] struct {
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
	TransformVersion int64

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

	// ToMemoryMinVersion and ToMemoryMaxVersion supply the bounds on
	// `TransformVersion` that `ToMemory` is compatible with, and must satisfy
	// `0 <= ToMemoryMinVersion <= TransformVersion <= ToMemoryMaxVersion`.
	//
	// To roll out a new `Transform` / `ToMemory` version without breaking active
	// frontend or cron processes:
	//  1. Increment `ToMemoryMaxVersion` and update `ToMemory` to support both
	//     old and new versions.
	//  2. Increment `TransformVersion` and update `Transform`.
	//  3. Increment `ToMemoryMinVersion` and drop old version support from
	//     `ToMemory`.
	//
	// NOTE: Failing to roll out the pipeline in this fashion will block server
	// startup!
	//
	// If the stored StoreMsg has a TransformVersion not in the range
	// [ToMemoryMinVersion, ToMemoryMaxVersion], the in-memory refresh will error
	// and will not invoke ToMemory.
	ToMemoryMinVersion int64
	ToMemoryMaxVersion int64

	// ToMemory, if supplied, transforms `StoreMsg` to `InMemory` any time a
	// project's transformed config is updated in storage.
	//
	// When a project's config is deleted from storage, the project's entry is
	// deleted from the in-memory map (causing [RegisteredProjects.GetData] to
	// return `MemoryDefault(project)`).
	//
	// The underlying store for the pipeline must implement [GettableStore], or
	// this config cannot be registered.
	//
	// If StoreMsg == InMemory, a default implementation will be provided which
	// passes through StoreMsg.
	ToMemory ToMemoryFunc[StoreMsg, InMemory]

	// MemoryDefault will be used when returning data for projects which have
	// no loaded config.
	//
	// If omitted, then the default value will be the zero value of InMemory.
	MemoryDefault func(project string) InMemory
}

func (pp *ProjectPipeline[StoreMsg, InMemory]) String() string {
	return fmt.Sprintf("ProjectPipeline{ID: %q}", pp.ID)
}

// Register registers this pipeline with the given registry.
//
// Errors are buffered until [Registry.Initialize] is called. Panics if r is
// already initialized. If using the Registry with the server Module, this must
// be called before server.Main, because the registered config implies module
// Dependencies.
//
// Returns nil if InMemory is [NoMemoryRefresh].
//
// Registering enables the pipeline when the Registry is used with the server:
//   - Validation handlers will call TransformFunc.
//   - Cron job will fetch and transform committed configs.
//   - In memory refresh will call ToMemory and cache accordingly.
//
// If `r` is nil, this uses [Default].
func (pp *ProjectPipeline[StoreMsg, InMemory]) Register(r *Registry) *RegisteredProjects[InMemory] {
	ret := &RegisteredProjects[InMemory]{
		ingestConfig: &ingestConfig{
			id:                 pp.ID,
			desc:               pp.String(),
			transformVersion:   pp.TransformVersion,
			toMemoryMinVersion: pp.ToMemoryMinVersion,
			toMemoryMaxVersion: pp.ToMemoryMaxVersion,
		},
		memoryDefault: pp.MemoryDefault,
	}
	store := func(cs config.Set, val *InMemory) {
		proj := cs.Project()
		if proj == "" {
			panic(fmt.Sprintf("impossible: config.Set %q with no project", cs))
		}
		if val == nil {
			ret.dataMap.Delete(proj)
		} else {
			ret.dataMap.Store(proj, *val)
		}
	}
	register(r, pp.Files, pp.Transform, pp.Store, pp.ToMemory, store, ret.ingestConfig)
	if reflect.TypeFor[InMemory]() == reflect.TypeFor[NoMemoryRefresh]() {
		return nil
	}
	return ret
}

// RegisteredProjects is a [ProjectPipeline] registered with a specific
// [Registry].
type RegisteredProjects[InMemory any] struct {
	ingestConfig *ingestConfig

	memoryDefault func(string) InMemory

	// dataMap maps project name (string) to InMemory, updated by the ToMemory
	// callback.
	dataMap sync.Map
}

// String returns the pipeline identifier description for this config.
func (ap *RegisteredProjects[InMemory]) String() string {
	if ap == nil {
		return "RegisteredProjects[nil]"
	}
	return fmt.Sprintf("RegisteredProjects[%s]", ap.ingestConfig)
}

// GetData returns the in-memory representation of the config for `project`,
// ensuring the in-memory data has been refreshed within
// [Registry.MaxMemoryStaleness].
//
// If the project's config does not exist in storage, returns
// `MemoryDefault(project)` (or the zero value of `InMemory` if `MemoryDefault`
// is nil) and a nil error.
//
// Panics if `ap` is nil or `InMemory` is [NoMemoryRefresh].
//
// All errors are tagged as transient.
func (ap *RegisteredProjects[InMemory]) GetData(ctx context.Context, project string) (InMemory, error) {
	var zero InMemory

	if err := ap.checkFresh(ctx, "GetData"); err != nil {
		return zero, err
	}

	ret, ok := ap.dataMap.Load(project)
	if !ok {
		if ap.memoryDefault != nil {
			return ap.memoryDefault(project), nil
		}
		return zero, nil
	}
	return ret.(InMemory), nil
}

// GetAllData returns the in-memory representation of the config for all given
// projects ensuring the in-memory data has been refreshed within
// [Registry.MaxMemoryStaleness].
//
// If a project's config does not exist in storage, populates
// `MemoryDefault(project)` when `MemoryDefault` is non-nil, or omits the
// project from the returned map when `MemoryDefault` is nil.
//
// Panics if `ap` is nil or `InMemory` is [NoMemoryRefresh].
//
// All errors are tagged as transient.
func (ap *RegisteredProjects[InMemory]) GetAllData(ctx context.Context, projects []string) (map[string]InMemory, error) {
	if err := ap.checkFresh(ctx, "GetAllData"); err != nil {
		return nil, err
	}

	ret := make(map[string]InMemory, len(projects))
	for _, project := range projects {
		if item, ok := ap.dataMap.Load(project); ok {
			ret[project] = item.(InMemory)
		} else if ap.memoryDefault != nil {
			ret[project] = ap.memoryDefault(project)
		}
	}
	return ret, nil
}

// checkFresh panics if this RegisteredProjects is not initialized, or
// returns an error if we couldn't get the data fresh enough for GetData or
// GetAllData.
func (ap *RegisteredProjects[InMemory]) checkFresh(ctx context.Context, method string) error {
	if reflect.TypeFor[InMemory]() == reflect.TypeFor[NoMemoryRefresh]() {
		panic(errors.Fmt("%s.%s not available: InMemory is NoMemoryRefresh", ap, method))
	}
	if ap == nil {
		panic(errors.Fmt("%s.%s not available: receiver is nil", ap, method))
	}
	return transient.Tag.Apply(ap.ingestConfig.checkFresh(ctx))
}
