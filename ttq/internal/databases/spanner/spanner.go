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

// Package spanner implements TTQ Database backend on top of Cloud Spanner.
package spanner

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/ttq/internal"
)

type DB struct {
	Client *spanner.Client
}

var _ internal.Database = (*DB)(nil)

// Kind is used only for monitoring/logging purposes.
func (d *DB) Kind() string {
	return "spanner"
}

func (d *DB) SaveReminder(_ context.Context, _ *internal.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (d *DB) DeleteReminder(_ context.Context, _ *internal.Reminder) error {
	panic("not implemented") // TODO: Implement
}
