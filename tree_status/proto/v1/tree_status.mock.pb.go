// Code generated by MockGen. DO NOT EDIT.
// Source: tree_status.pb.go

package v1

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	grpc "google.golang.org/grpc"
)

// MockTreeStatusClient is a mock of TreeStatusClient interface.
type MockTreeStatusClient struct {
	ctrl     *gomock.Controller
	recorder *MockTreeStatusClientMockRecorder
}

// MockTreeStatusClientMockRecorder is the mock recorder for MockTreeStatusClient.
type MockTreeStatusClientMockRecorder struct {
	mock *MockTreeStatusClient
}

// NewMockTreeStatusClient creates a new mock instance.
func NewMockTreeStatusClient(ctrl *gomock.Controller) *MockTreeStatusClient {
	mock := &MockTreeStatusClient{ctrl: ctrl}
	mock.recorder = &MockTreeStatusClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTreeStatusClient) EXPECT() *MockTreeStatusClientMockRecorder {
	return m.recorder
}

// CreateStatus mocks base method.
func (m *MockTreeStatusClient) CreateStatus(ctx context.Context, in *CreateStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CreateStatus", varargs...)
	ret0, _ := ret[0].(*Status)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateStatus indicates an expected call of CreateStatus.
func (mr *MockTreeStatusClientMockRecorder) CreateStatus(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateStatus", reflect.TypeOf((*MockTreeStatusClient)(nil).CreateStatus), varargs...)
}

// GetStatus mocks base method.
func (m *MockTreeStatusClient) GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "GetStatus", varargs...)
	ret0, _ := ret[0].(*Status)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetStatus indicates an expected call of GetStatus.
func (mr *MockTreeStatusClientMockRecorder) GetStatus(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetStatus", reflect.TypeOf((*MockTreeStatusClient)(nil).GetStatus), varargs...)
}

// ListStatus mocks base method.
func (m *MockTreeStatusClient) ListStatus(ctx context.Context, in *ListStatusRequest, opts ...grpc.CallOption) (*ListStatusResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "ListStatus", varargs...)
	ret0, _ := ret[0].(*ListStatusResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListStatus indicates an expected call of ListStatus.
func (mr *MockTreeStatusClientMockRecorder) ListStatus(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListStatus", reflect.TypeOf((*MockTreeStatusClient)(nil).ListStatus), varargs...)
}

// MockTreeStatusServer is a mock of TreeStatusServer interface.
type MockTreeStatusServer struct {
	ctrl     *gomock.Controller
	recorder *MockTreeStatusServerMockRecorder
}

// MockTreeStatusServerMockRecorder is the mock recorder for MockTreeStatusServer.
type MockTreeStatusServerMockRecorder struct {
	mock *MockTreeStatusServer
}

// NewMockTreeStatusServer creates a new mock instance.
func NewMockTreeStatusServer(ctrl *gomock.Controller) *MockTreeStatusServer {
	mock := &MockTreeStatusServer{ctrl: ctrl}
	mock.recorder = &MockTreeStatusServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTreeStatusServer) EXPECT() *MockTreeStatusServerMockRecorder {
	return m.recorder
}

// CreateStatus mocks base method.
func (m *MockTreeStatusServer) CreateStatus(arg0 context.Context, arg1 *CreateStatusRequest) (*Status, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateStatus", arg0, arg1)
	ret0, _ := ret[0].(*Status)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateStatus indicates an expected call of CreateStatus.
func (mr *MockTreeStatusServerMockRecorder) CreateStatus(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateStatus", reflect.TypeOf((*MockTreeStatusServer)(nil).CreateStatus), arg0, arg1)
}

// GetStatus mocks base method.
func (m *MockTreeStatusServer) GetStatus(arg0 context.Context, arg1 *GetStatusRequest) (*Status, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetStatus", arg0, arg1)
	ret0, _ := ret[0].(*Status)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetStatus indicates an expected call of GetStatus.
func (mr *MockTreeStatusServerMockRecorder) GetStatus(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetStatus", reflect.TypeOf((*MockTreeStatusServer)(nil).GetStatus), arg0, arg1)
}

// ListStatus mocks base method.
func (m *MockTreeStatusServer) ListStatus(arg0 context.Context, arg1 *ListStatusRequest) (*ListStatusResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListStatus", arg0, arg1)
	ret0, _ := ret[0].(*ListStatusResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListStatus indicates an expected call of ListStatus.
func (mr *MockTreeStatusServerMockRecorder) ListStatus(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListStatus", reflect.TypeOf((*MockTreeStatusServer)(nil).ListStatus), arg0, arg1)
}