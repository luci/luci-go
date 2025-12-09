// Copyright 2025 The LUCI Authors.
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

package gs

import (
	"context"
	"reflect"

	gs "cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
)

// MockGcsClient is a mock of gcsClient interface
type MockGcsClient struct {
	ctrl     *gomock.Controller
	recorder *MockGcsClientMockRecorder
}

// MockGcsClientMockRecorder is the mock recorder for MockGcsClient
type MockGcsClientMockRecorder struct {
	mock *MockGcsClient
}

// NewMockGcsClient creates a new MockGcsClient instance
func NewMockGcsClient(ctrl *gomock.Controller) *MockGcsClient {
	mock := &MockGcsClient{ctrl: ctrl}
	mock.recorder = &MockGcsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockGcsClient) EXPECT() *MockGcsClientMockRecorder {
	return m.recorder
}

// Bucket mocks base method
func (m *MockGcsClient) Bucket(name string) gcsBucketHandle {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Bucket", name)
	ret0, _ := ret[0].(gcsBucketHandle)
	return ret0
}

// Bucket indicates an expected call of Bucket
func (mr *MockGcsClientMockRecorder) Bucket(name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Bucket", reflect.TypeOf((*MockGcsClient)(nil).Bucket), name)
}

// Close mocks base method
func (m *MockGcsClient) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockGcsClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockGcsClient)(nil).Close))
}

// MockGcsBucketHandle is a mock of gcsBucketHandle interface
type MockGcsBucketHandle struct {
	ctrl     *gomock.Controller
	recorder *MockGcsBucketHandleMockRecorder
}

// MockGcsBucketHandleMockRecorder is the mock recorder for MockGcsBucketHandle
type MockGcsBucketHandleMockRecorder struct {
	mock *MockGcsBucketHandle
}

// NewMockGcsBucketHandle creates a new mock instance
func NewMockGcsBucketHandle(ctrl *gomock.Controller) *MockGcsBucketHandle {
	mock := &MockGcsBucketHandle{ctrl: ctrl}
	mock.recorder = &MockGcsBucketHandleMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockGcsBucketHandle) EXPECT() *MockGcsBucketHandleMockRecorder {
	return m.recorder
}

// Object mocks base method
func (m *MockGcsBucketHandle) Object(name string) *gs.ObjectHandle {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Object", name)
	ret0, _ := ret[0].(*gs.ObjectHandle)
	return ret0
}

// Object indicates an expected call of Object
func (mr *MockGcsBucketHandleMockRecorder) Object(name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Object", reflect.TypeOf((*MockGcsBucketHandle)(nil).Object), name)
}

// Objects mocks base method
func (m *MockGcsBucketHandle) Objects(ctx context.Context, q *gs.Query) gcsObjectIterator {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Objects", ctx, q)
	ret0, _ := ret[0].(gcsObjectIterator)
	return ret0
}

// Objects indicates an expected call of Objects
func (mr *MockGcsBucketHandleMockRecorder) Objects(ctx, q interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Objects", reflect.TypeOf((*MockGcsBucketHandle)(nil).Objects), ctx, q)
}

// SignedURL mocks base method
func (m *MockGcsBucketHandle) SignedURL(name string, opts *gs.SignedURLOptions) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SignedURL", name, opts)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SignedURL indicates an expected call of SignedURL
func (mr *MockGcsBucketHandleMockRecorder) SignedURL(name, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SignedURL", reflect.TypeOf((*MockGcsBucketHandle)(nil).SignedURL), name, opts)
}

// MockGcsObjectIterator is a mock of gcsObjectIterator interface
type MockGcsObjectIterator struct {
	ctrl     *gomock.Controller
	recorder *MockGcsObjectIteratorMockRecorder
}

// MockGcsObjectIteratorMockRecorder is the mock recorder for MockGcsObjectIterator
type MockGcsObjectIteratorMockRecorder struct {
	mock *MockGcsObjectIterator
}

// NewMockGcsObjectIterator creates a new mock instance
func NewMockGcsObjectIterator(ctrl *gomock.Controller) *MockGcsObjectIterator {
	mock := &MockGcsObjectIterator{ctrl: ctrl}
	mock.recorder = &MockGcsObjectIteratorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockGcsObjectIterator) EXPECT() *MockGcsObjectIteratorMockRecorder {
	return m.recorder
}

// Next mocks base method
func (m *MockGcsObjectIterator) Next() (*gs.ObjectAttrs, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next")
	ret0, _ := ret[0].(*gs.ObjectAttrs)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Next indicates an expected call of Next
func (mr *MockGcsObjectIteratorMockRecorder) Next() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockGcsObjectIterator)(nil).Next))
}
