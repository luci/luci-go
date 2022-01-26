// Copyright 2022 The LUCI Authors.
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

// Package quotaconfig exports the interface required by the quota library to
// read *pb.Policy configs. Provides an in-memory implementation of the
// interface.
package quotaconfig

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/server/quota/proto"
)

// Interface encapsulates the functionality needed to implement a configuration
// layer usable by the quota library.
type Interface interface {
	// Get returns the named *pb.Policy.
	//
	// Called by the quota library every time quota is manipulated,
	// so implementations should return relatively quickly.
	Get(context.Context, string) (*pb.Policy, error)
}

// Ensure Memory implements Interface at compile-time.
var _ Interface = &Memory{}

// Memory holds known *pb.Policy protos in memory.
// Implements Interface. Safe for concurrent use.
type Memory struct {
	// lock is a mutex governing reads/writes to policies.
	lock sync.RWMutex

	// policies is a map of policy name to *pb.Policy with that name.
	policies map[string]*pb.Policy
}

// Get returns a copy of the named *pb.Policy if it exists.
func (m *Memory) Get(ctx context.Context, name string) (*pb.Policy, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	p, ok := m.policies[name]
	if !ok {
		return nil, errors.Reason("policy %q not found", name).Err()
	}
	return proto.Clone(p).(*pb.Policy), nil
}

// NewMemory returns a new Memory initialized with the given policies.
func NewMemory(policies []*pb.Policy) Interface {
	m := &Memory{
		policies: make(map[string]*pb.Policy, len(policies)),
	}
	for _, p := range policies {
		m.policies[p.Name] = p
	}
	return m
}
