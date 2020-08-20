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

package txnBuf

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
)

type dsBuf struct {
	ds.RawInterface
}

var _ ds.RawInterface = (*dsBuf)(nil)

func (d *dsBuf) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	return doRunInTransaction(d.RawInterface, f, opts)
}

func doRunInTransaction(base ds.RawInterface, f func(context.Context) error, opts *ds.TransactionOptions) error {
	return base.RunInTransaction(func(ctx context.Context) error {
		return withTxnBuf(ctx, f, opts)
	}, opts)
}
