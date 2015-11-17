// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"golang.org/x/net/context"
)

// FwdEdge represents a forward-dependency from one attempt to another. The
// From attempt will block until the To attempt completes.
type FwdEdge struct {
	From *types.AttemptID
	To   *types.AttemptID
}

// Fwd returns the Attempt (From) and FwdDep (To) models that this FwdEdge
// represents.
func (e *FwdEdge) Fwd(c context.Context) (*Attempt, *FwdDep) {
	atmpt := &Attempt{AttemptID: *e.From}
	return atmpt, &FwdDep{
		Depender: datastore.Get(c).KeyForObj(atmpt),
		Dependee: *e.To,
	}
}

// Back returns the BackDepGroup (To) and the BackDep (From) models that
// represent the reverse dependency for this FwdEdge.
func (e *FwdEdge) Back(c context.Context) (*BackDepGroup, *BackDep) {
	bdg := &BackDepGroup{Dependee: *e.To}
	return bdg, &BackDep{
		DependeeGroup: datastore.Get(c).KeyForObj(bdg),
		Depender:      *e.From,
	}
}

// AttemptFanout is a one-to-many set of FwdEdges. It's used to efficiently
// represent a single Attempt depending on multiple other Attempts.
type AttemptFanout struct {
	Base  *types.AttemptID
	Edges types.AttemptIDSlice
}

// Fwds returns a slice of all the FwdDep models represented by this
// AttemptFanout.
func (f *AttemptFanout) Fwds(c context.Context) []*FwdDep {
	from := datastore.Get(c).KeyForObj(&Attempt{AttemptID: *f.Base})
	ret := make([]*FwdDep, len(f.Edges))
	for i, aid := range f.Edges {
		ret[i] = &FwdDep{Depender: from, Dependee: *aid}
	}
	return ret
}
