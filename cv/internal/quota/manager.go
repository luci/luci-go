// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quota

import (
	"context"
	"sync"

	srvquota "go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
)

var qinit sync.Once
var qapp *srvquota.Application

// Manager manages the quota accounts for CV users.
type Manager struct {
	qapp *srvquota.Application
}

// DebitRunQuota debits the run quota from a given user's account.
func (qm *Manager) DebitRunQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// CreditRunQuota credits the run quota into a given user's account.
func (qm *Manager) CreditRunQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// DebitTryjobQuota debits the tryjob quota from a given user's account.
func (qm *Manager) DebitTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// CreditTryjobQuota credits the tryjob quota into a given user's account.
func (qm *Manager) CreditTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// NewManager creates a new quota manager.
func NewManager() *Manager {
	qinit.Do(func() {
		qapp = srvquota.Register("cv", &srvquota.ApplicationOptions{
			ResourceTypes: []string{"runs", "tryjobs"},
		})
	})
	return &Manager{qapp: qapp}
}
