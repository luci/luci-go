// Copyright 2021 The LUCI Authors.
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

// Package treetest implements fake Tree for testing in CV.
package treetest

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/prpctest"
	tspb "go.chromium.org/luci/tree_status/proto/v1"
)

// FakeServer simulates tree status server used in test.
type FakeServer struct {
	tspb.UnimplementedTreeStatusServer
	prpcSrv     *prpctest.Server
	stateByTree map[string]tspb.GeneralState
	errorByTree map[string]error
	// mu protects access/mutation to this fake Tree.
	mu sync.RWMutex
}

// NewFakeServer returns a fake tree status server.
//
// `Shutdown` SHOULD be called after test completes.
func NewFakeServer(ctx context.Context) *FakeServer {
	prpcSrv := &prpctest.Server{}
	fs := &FakeServer{
		prpcSrv:     prpcSrv,
		stateByTree: make(map[string]tspb.GeneralState),
		errorByTree: make(map[string]error),
	}
	tspb.RegisterTreeStatusServer(prpcSrv, fs)
	prpcSrv.Start(ctx)
	return fs
}

// GetStatus implements `tspb.TreeStatusServer`.
func (fs *FakeServer) GetStatus(ctx context.Context, req *tspb.GetStatusRequest) (*tspb.Status, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if err, ok := fs.errorByTree[req.GetName()]; ok && err != nil {
		return nil, err
	}
	if state, ok := fs.stateByTree[req.GetName()]; ok {
		return &tspb.Status{
			Name:         req.GetName(),
			GeneralState: state,
		}, nil
	}
	return nil, status.Errorf(codes.NotFound, "tree %q not found", req.GetName())
}

// Host returns the address of fake tree status server.
func (fs *FakeServer) Host() string {
	if fs.prpcSrv == nil {
		panic(errors.New("fake tree status server is not initialized"))
	}
	return fs.prpcSrv.Host
}

// Shutdown closes the fake tree status server.
func (fs *FakeServer) Shutdown() {
	if fs.prpcSrv != nil {
		fs.prpcSrv.Close()
	}
}

// InjectErr makes Tree status server return error for the given tree.
//
// Passing nil error will bring the tree back to normal.
func (fs *FakeServer) InjectErr(treeName string, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.errorByTree[fmt.Sprintf("trees/%s/status/latest", treeName)] = err
}

// ModifyState sets the state of the given tree.
func (fs *FakeServer) ModifyState(treeName string, state tspb.GeneralState) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.stateByTree[fmt.Sprintf("trees/%s/status/latest", treeName)] = state
}
