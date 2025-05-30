// Code generated by MockGen. DO NOT EDIT.
// Source: gitiles.pb.go

// Package mock_gitiles is a generated GoMock package.
package mock_gitiles

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	gitiles "go.chromium.org/luci/common/proto/gitiles"
	grpc "google.golang.org/grpc"
)

// MockGitilesClient is a mock of GitilesClient interface.
type MockGitilesClient struct {
	ctrl     *gomock.Controller
	recorder *MockGitilesClientMockRecorder
}

// MockGitilesClientMockRecorder is the mock recorder for MockGitilesClient.
type MockGitilesClientMockRecorder struct {
	mock *MockGitilesClient
}

// NewMockGitilesClient creates a new mock instance.
func NewMockGitilesClient(ctrl *gomock.Controller) *MockGitilesClient {
	mock := &MockGitilesClient{ctrl: ctrl}
	mock.recorder = &MockGitilesClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGitilesClient) EXPECT() *MockGitilesClientMockRecorder {
	return m.recorder
}

// Archive mocks base method.
func (m *MockGitilesClient) Archive(ctx context.Context, in *gitiles.ArchiveRequest, opts ...grpc.CallOption) (*gitiles.ArchiveResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Archive", varargs...)
	ret0, _ := ret[0].(*gitiles.ArchiveResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Archive indicates an expected call of Archive.
func (mr *MockGitilesClientMockRecorder) Archive(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Archive", reflect.TypeOf((*MockGitilesClient)(nil).Archive), varargs...)
}

// DownloadDiff mocks base method.
func (m *MockGitilesClient) DownloadDiff(ctx context.Context, in *gitiles.DownloadDiffRequest, opts ...grpc.CallOption) (*gitiles.DownloadDiffResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DownloadDiff", varargs...)
	ret0, _ := ret[0].(*gitiles.DownloadDiffResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DownloadDiff indicates an expected call of DownloadDiff.
func (mr *MockGitilesClientMockRecorder) DownloadDiff(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadDiff", reflect.TypeOf((*MockGitilesClient)(nil).DownloadDiff), varargs...)
}

// DownloadFile mocks base method.
func (m *MockGitilesClient) DownloadFile(ctx context.Context, in *gitiles.DownloadFileRequest, opts ...grpc.CallOption) (*gitiles.DownloadFileResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DownloadFile", varargs...)
	ret0, _ := ret[0].(*gitiles.DownloadFileResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DownloadFile indicates an expected call of DownloadFile.
func (mr *MockGitilesClientMockRecorder) DownloadFile(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadFile", reflect.TypeOf((*MockGitilesClient)(nil).DownloadFile), varargs...)
}

// GetProject mocks base method.
func (m *MockGitilesClient) GetProject(ctx context.Context, in *gitiles.GetProjectRequest, opts ...grpc.CallOption) (*gitiles.Project, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "GetProject", varargs...)
	ret0, _ := ret[0].(*gitiles.Project)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetProject indicates an expected call of GetProject.
func (mr *MockGitilesClientMockRecorder) GetProject(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetProject", reflect.TypeOf((*MockGitilesClient)(nil).GetProject), varargs...)
}

// ListFiles mocks base method.
func (m *MockGitilesClient) ListFiles(ctx context.Context, in *gitiles.ListFilesRequest, opts ...grpc.CallOption) (*gitiles.ListFilesResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "ListFiles", varargs...)
	ret0, _ := ret[0].(*gitiles.ListFilesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListFiles indicates an expected call of ListFiles.
func (mr *MockGitilesClientMockRecorder) ListFiles(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListFiles", reflect.TypeOf((*MockGitilesClient)(nil).ListFiles), varargs...)
}

// Log mocks base method.
func (m *MockGitilesClient) Log(ctx context.Context, in *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Log", varargs...)
	ret0, _ := ret[0].(*gitiles.LogResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Log indicates an expected call of Log.
func (mr *MockGitilesClientMockRecorder) Log(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Log", reflect.TypeOf((*MockGitilesClient)(nil).Log), varargs...)
}

// Projects mocks base method.
func (m *MockGitilesClient) Projects(ctx context.Context, in *gitiles.ProjectsRequest, opts ...grpc.CallOption) (*gitiles.ProjectsResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Projects", varargs...)
	ret0, _ := ret[0].(*gitiles.ProjectsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Projects indicates an expected call of Projects.
func (mr *MockGitilesClientMockRecorder) Projects(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Projects", reflect.TypeOf((*MockGitilesClient)(nil).Projects), varargs...)
}

// Refs mocks base method.
func (m *MockGitilesClient) Refs(ctx context.Context, in *gitiles.RefsRequest, opts ...grpc.CallOption) (*gitiles.RefsResponse, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Refs", varargs...)
	ret0, _ := ret[0].(*gitiles.RefsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Refs indicates an expected call of Refs.
func (mr *MockGitilesClientMockRecorder) Refs(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Refs", reflect.TypeOf((*MockGitilesClient)(nil).Refs), varargs...)
}

// MockGitilesServer is a mock of GitilesServer interface.
type MockGitilesServer struct {
	ctrl     *gomock.Controller
	recorder *MockGitilesServerMockRecorder
}

// MockGitilesServerMockRecorder is the mock recorder for MockGitilesServer.
type MockGitilesServerMockRecorder struct {
	mock *MockGitilesServer
}

// NewMockGitilesServer creates a new mock instance.
func NewMockGitilesServer(ctrl *gomock.Controller) *MockGitilesServer {
	mock := &MockGitilesServer{ctrl: ctrl}
	mock.recorder = &MockGitilesServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGitilesServer) EXPECT() *MockGitilesServerMockRecorder {
	return m.recorder
}

// Archive mocks base method.
func (m *MockGitilesServer) Archive(arg0 context.Context, arg1 *gitiles.ArchiveRequest) (*gitiles.ArchiveResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Archive", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.ArchiveResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Archive indicates an expected call of Archive.
func (mr *MockGitilesServerMockRecorder) Archive(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Archive", reflect.TypeOf((*MockGitilesServer)(nil).Archive), arg0, arg1)
}

// DownloadDiff mocks base method.
func (m *MockGitilesServer) DownloadDiff(arg0 context.Context, arg1 *gitiles.DownloadDiffRequest) (*gitiles.DownloadDiffResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DownloadDiff", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.DownloadDiffResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DownloadDiff indicates an expected call of DownloadDiff.
func (mr *MockGitilesServerMockRecorder) DownloadDiff(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadDiff", reflect.TypeOf((*MockGitilesServer)(nil).DownloadDiff), arg0, arg1)
}

// DownloadFile mocks base method.
func (m *MockGitilesServer) DownloadFile(arg0 context.Context, arg1 *gitiles.DownloadFileRequest) (*gitiles.DownloadFileResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DownloadFile", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.DownloadFileResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DownloadFile indicates an expected call of DownloadFile.
func (mr *MockGitilesServerMockRecorder) DownloadFile(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadFile", reflect.TypeOf((*MockGitilesServer)(nil).DownloadFile), arg0, arg1)
}

// GetProject mocks base method.
func (m *MockGitilesServer) GetProject(arg0 context.Context, arg1 *gitiles.GetProjectRequest) (*gitiles.Project, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetProject", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.Project)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetProject indicates an expected call of GetProject.
func (mr *MockGitilesServerMockRecorder) GetProject(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetProject", reflect.TypeOf((*MockGitilesServer)(nil).GetProject), arg0, arg1)
}

// ListFiles mocks base method.
func (m *MockGitilesServer) ListFiles(arg0 context.Context, arg1 *gitiles.ListFilesRequest) (*gitiles.ListFilesResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListFiles", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.ListFilesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListFiles indicates an expected call of ListFiles.
func (mr *MockGitilesServerMockRecorder) ListFiles(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListFiles", reflect.TypeOf((*MockGitilesServer)(nil).ListFiles), arg0, arg1)
}

// Log mocks base method.
func (m *MockGitilesServer) Log(arg0 context.Context, arg1 *gitiles.LogRequest) (*gitiles.LogResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Log", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.LogResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Log indicates an expected call of Log.
func (mr *MockGitilesServerMockRecorder) Log(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Log", reflect.TypeOf((*MockGitilesServer)(nil).Log), arg0, arg1)
}

// Projects mocks base method.
func (m *MockGitilesServer) Projects(arg0 context.Context, arg1 *gitiles.ProjectsRequest) (*gitiles.ProjectsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Projects", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.ProjectsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Projects indicates an expected call of Projects.
func (mr *MockGitilesServerMockRecorder) Projects(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Projects", reflect.TypeOf((*MockGitilesServer)(nil).Projects), arg0, arg1)
}

// Refs mocks base method.
func (m *MockGitilesServer) Refs(arg0 context.Context, arg1 *gitiles.RefsRequest) (*gitiles.RefsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Refs", arg0, arg1)
	ret0, _ := ret[0].(*gitiles.RefsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Refs indicates an expected call of Refs.
func (mr *MockGitilesServerMockRecorder) Refs(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Refs", reflect.TypeOf((*MockGitilesServer)(nil).Refs), arg0, arg1)
}
