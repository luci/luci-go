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
// interface suitable for testing. See quotaconfig subpackages for other
// implementations.
package quotaconfig

import (
	"context"
	"regexp"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/server/quotabeta/proto"
)

// ErrNotFound must be returned by Interface.Get implementations when the named
// *pb.Policy is not found.
var ErrNotFound = errors.New("policy not found")

// Interface encapsulates the functionality needed to implement a configuration
// layer usable by the quota library. Implementations should ensure returned
// *pb.Policies are valid (see ValidatePolicy).
type Interface interface {
	// Get returns the named *pb.Policy or ErrNotFound if it doesn't exist.
	//
	// Called by the quota library every time quota is manipulated,
	// so implementations should return relatively quickly.
	Get(context.Context, string) (*pb.Policy, error)

	// Refresh fetches all *pb.Policies.
	//
	// Implementations should validate (see ValidatePolicy) and cache configs so
	// future Get calls return relatively quickly.
	Refresh(context.Context) error
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

// Get returns a copy of the named *pb.Policy if it exists, or else ErrNotFound.
func (m *Memory) Get(ctx context.Context, name string) (*pb.Policy, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	p, ok := m.policies[name]
	if !ok {
		return nil, ErrNotFound
	}
	return proto.Clone(p).(*pb.Policy), nil
}

// Refresh fetches all *pb.Policies.
func (m *Memory) Refresh(ctx context.Context) error {
	return nil
}

// NewMemory returns a new Memory initialized with the given policies.
func NewMemory(ctx context.Context, policies []*pb.Policy) (Interface, error) {
	m := &Memory{
		policies: make(map[string]*pb.Policy, len(policies)),
	}
	v := &validation.Context{
		Context: ctx,
	}
	for i, p := range policies {
		p = proto.Clone(p).(*pb.Policy)
		v.Enter("policy %d", i)
		ValidatePolicy(v, p)
		m.policies[p.Name] = p
		v.Exit()
	}
	if err := v.Finalize(); err != nil {
		return nil, errors.Fmt("policies did not pass validation: %w", err)
	}
	return m, nil
}

// policyName is a *regexp.Regexp which policy names must match.
// Must start with a letter. Allowed characters (no spaces): A-Z a-z 0-9 - _ /
// The special substring ${user} is allowed. Must not exceed 64 characters.
var policyName = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9-_/]|(\$\{user\}))*$`)

// ValidatePolicy validates the given *pb.Policy.
func ValidatePolicy(ctx *validation.Context, p *pb.Policy) {
	if !policyName.MatchString(p.GetName()) {
		ctx.Errorf("name must match %q", policyName.String())
	}
	if len(p.GetName()) > 64 {
		ctx.Errorf("name must not exceed 64 characters")
	}
	if p.GetResources() < 0 {
		ctx.Errorf("resources must not be negative")
	}
	if p.GetReplenishment() < 0 {
		ctx.Errorf("replenishment must not be negative")
	}
}
