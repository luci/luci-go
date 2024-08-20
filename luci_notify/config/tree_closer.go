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

package config

import (
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

type TreeCloserStatus string

const (
	Open   TreeCloserStatus = "Open"
	Closed TreeCloserStatus = "Closed"
)

// TreeCloser represents a tree closing rule from the config, along with its
// current state.
type TreeCloser struct {
	// BuilderKey is a datastore key to this TreeCloser's parent builder.
	BuilderKey *datastore.Key `gae:"$parent"`

	// TreeName is the tree that this rule opens/closes. This is
	// duplicated from notifypb.TreeCloser, so that we can use it as the ID
	// for datastore. The combination of tree name and builder is
	// guaranteed to be unique.
	TreeName string `gae:"$id"`

	// TreeCloser is the underlying TreeCloser proto from the current
	// version of the config.
	TreeCloser notifypb.TreeCloser

	// Status is the current status of this rule. If any TreeCloser for a
	// given tree-status host has a status of 'Closed', the tree will be
	// closed.
	Status TreeCloserStatus

	// Timestamp stores the finish time of the build which caused us to set
	// the current status. This is used to decide which template to use
	// when setting the tree status message.
	Timestamp time.Time

	// Message contains the status message to use if this TreeCloser is the
	// one that updates the status of the tree. Only valid if Status is
	// Closed.
	Message string
}
