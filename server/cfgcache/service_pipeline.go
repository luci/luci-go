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
	"cmp"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// PassthroughServicePipeline is a shorthand for a [ServicePipeline] whose
// stored and in-memory types are identical.
type PassthroughServicePipeline[T proto.Message] = ServicePipeline[T, T]

// ServicePipeline describes a pipeline to consume a snapshot of one or more
// config files for a single service, usually the current one.
//
// This pipeline will validate the snapshot and save it as `StoreMsg`.
//
// This specifically uses the config set pattern of
// `exact:services/{ServiceName}`, with the service name defaulting to
// `${appid}`.
type ServicePipeline[StoreMsg proto.Message, InMemory any] struct {
	// ID uniquely identifies this pipeline within a [Registry] and in storage.
	//
	// Required.
	ID string

	// Store is the required storage for storing the transformed configuration.
	//
	// See 'dsstore' for a datastore-backed implementation (or 'memstore' for
	// testing).
	Store Store[StoreMsg]

	// ServiceName is the name of the service to use.
	//
	// Defaults to "${appid}".
	//
	// The `appid` variable is set via the `cfgmodule` server module in
	// [go.chromium.org/luci/config/server/cfgmodule]. This is usually the name
	// of the Google Cloud Project, which is implicit for the AppEngine Classic
	// environment, or set via the `-cloud-project` server flag.
	ServiceName string

	// Files is one or more [*SingleFile] or [*RegexFile] entries to consume from
	// the service's config set.
	//
	// The Transform function will be invoked with a consistent [Snapshot] of all
	// of these when any of them change.
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
	// If the stored StoreMsg has a TransformVersion > ToMemoryMaxVersion, the
	// in-memory refresh loop will log a warning, and will not invoke ToMemory.
	//
	// If the stored StoreMsg has a TransformVersion < ToMemoryMinVersion, the
	// in-memory refresh loop will log an error, and will not invoke ToMemory.
	ToMemoryMinVersion int64
	ToMemoryMaxVersion int64

	// ToMemory, if supplied, transforms `StoreMsg` to `InMemory` any time the
	// transformed config is updated in storage.
	//
	// The returned value is cached (returned from [RegisteredService.GetData]).
	// If the config is deleted from storage, the cached value will be cleared
	// (causing [RegisteredService.GetData] to return `MemoryDefault()`).
	//
	// The underlying store for the pipeline must implement [GettableStore], or
	// this config cannot be registered.
	//
	// If StoreMsg == InMemory, a default implementation will be provided which
	// passes through StoreMsg.
	ToMemory ToMemoryFunc[StoreMsg, InMemory]

	// MemoryDefault will be used when returning data without a loaded config
	// (e.g. if the config does not exist yet).
	//
	// If omitted, then the default value will be the zero value of InMemory.
	MemoryDefault func() InMemory
}

func (s *ServicePipeline[StoreMsg, InMemory]) String() string {
	return fmt.Sprintf("ServicePipeline{ID: %q, ServiceName: %q}", s.ID, cmp.Or(s.ServiceName, "${appid}"))
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
//   - Cron job will fetch and transform committed configs.
func (s *ServicePipeline[StoreMsg, InMemory]) Register(r *Registry) {
	register(r, s.Files, s.Transform, s.Store, &ingestConfig{
		id:                 s.ID,
		desc:               s.String(),
		serviceNamePattern: cmp.Or(s.ServiceName, "${appid}"),
		transformVersion:   s.TransformVersion,
	})
}
