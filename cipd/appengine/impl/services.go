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

// Package impl instantiates the full implementation of the CIPD backend
// services.
//
// It is imported by GAE's frontend and backend modules that expose appropriate
// bits and pieces over pRPC and HTTP.
package impl

import (
	"go.chromium.org/luci/appengine/tq"

	"go.chromium.org/luci/cipd/appengine/impl/admin"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/repo"
)

var (
	// TQ is global Task Queue dispatcher used by the CIPD service.
	//
	// It serializes and routes Task Queue tasks. The tasks are registered in
	// the constructors below. The router is installed in 'backend' module only,
	// since we executed tasks only there.
	TQ = tq.Dispatcher{BaseURL: "/internal/tq/"}

	// InternalCAS is non-ACLed implementation of cas.StorageService to be used
	// only from within the backend code itself.
	InternalCAS = cas.Internal(&TQ)

	// PublicCAS is ACL-protected implementation of cas.StorageServer that can be
	// exposed as a public API.
	PublicCAS = cas.Public(InternalCAS)

	// PublicRepo is ACL-protected implementation of cipd.RepositoryServer that
	// can be exposed as a public API.
	PublicRepo = repo.Public(InternalCAS, &TQ)

	// AdminAPI is ACL-protected implementation of cipd.AdminServer that can be
	// exposed as an external API to be used by administrators.
	AdminAPI = admin.AdminAPI(&TQ)
)
