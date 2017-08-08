// Copyright 2015 The LUCI Authors.
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

package model

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/dm/api/service/v1"
)

// FwdEdge represents a forward-dependency from one attempt to another. The
// From attempt will block until the To attempt completes.
type FwdEdge struct {
	From *dm.Attempt_ID
	To   *dm.Attempt_ID
}

// Fwd returns the Attempt (From) and FwdDep (To) models that this FwdEdge
// represents.
func (e *FwdEdge) Fwd(c context.Context) (*Attempt, *FwdDep) {
	atmpt := &Attempt{ID: *e.From}
	return atmpt, &FwdDep{
		Depender: ds.KeyForObj(c, atmpt),
		Dependee: *e.To,
	}
}

// Back returns the BackDepGroup (To) and the BackDep (From) models that
// represent the reverse dependency for this FwdEdge.
func (e *FwdEdge) Back(c context.Context) (*BackDepGroup, *BackDep) {
	bdg := &BackDepGroup{Dependee: *e.To}
	return bdg, &BackDep{
		DependeeGroup: ds.KeyForObj(c, bdg),
		Depender:      *e.From,
	}
}
