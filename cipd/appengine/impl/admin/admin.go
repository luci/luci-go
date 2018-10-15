// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/appengine/mapper"
	"go.chromium.org/luci/appengine/tq"

	api "go.chromium.org/luci/cipd/api/admin/v1"
)

// adminImpl implements cipd.AdminServer, with an assumption that auth check has
// already been done by the decorator setup in AdminAPI(), see acl.go.
type adminImpl struct {
	tq  *tq.Dispatcher
	ctl *mapper.Controller
}

// init initializes mapper controller and registers mapping tasks.
func (impl *adminImpl) init() {
	impl.ctl = &mapper.Controller{
		MapperQueue:  "mappers", // see queue.yaml
		ControlQueue: "default",
	}
	for _, m := range mappers { // see mappers.go
		impl.ctl.RegisterFactory(m.mapperID(), m.newMapper)
	}
	impl.ctl.Install(impl.tq)
}

// toStatus converts an error from mapper.Controller to an grpc status.
//
// Passes nil as is.
func toStatus(err error) error {
	switch {
	case err == mapper.ErrNoSuchJob:
		return status.Errorf(codes.NotFound, "no such mapping job")
	case transient.Tag.In(err):
		return status.Errorf(codes.Internal, err.Error())
	case err != nil:
		return status.Errorf(codes.InvalidArgument, err.Error())
	default:
		return nil
	}
}

// LaunchJob implements the corresponding RPC method, see the proto doc.
func (impl *adminImpl) LaunchJob(c context.Context, cfg *api.JobConfig) (*api.JobID, error) {
	def, ok := mappers[cfg.Kind] // see mappers.go
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown mapper kind")
	}

	cfgBlob, err := proto.Marshal(cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal JobConfig - %s", err)
	}

	launchCfg := def.Config
	launchCfg.Mapper = def.mapperID()
	launchCfg.Params = cfgBlob

	jid, err := impl.ctl.LaunchJob(c, &launchCfg)
	if err != nil {
		return nil, toStatus(err)
	}

	return &api.JobID{JobId: int64(jid)}, nil
}

// AbortJob implements the corresponding RPC method, see the proto doc.
func (impl *adminImpl) AbortJob(c context.Context, id *api.JobID) (*empty.Empty, error) {
	_, err := impl.ctl.AbortJob(c, mapper.JobID(id.JobId))
	if err != nil {
		return nil, toStatus(err)
	}
	return &empty.Empty{}, nil
}

// GetJobState implements the corresponding RPC method, see the proto doc.
func (impl *adminImpl) GetJobState(c context.Context, id *api.JobID) (*api.JobState, error) {
	job, err := impl.ctl.GetJob(c, mapper.JobID(id.JobId))
	if err != nil {
		return nil, toStatus(err)
	}
	cfg := &api.JobConfig{}
	if err := proto.Unmarshal(job.Config.Params, cfg); err != nil {
		return nil, toStatus(errors.Annotate(err, "failed to unmarshal JobConfig").Err())
	}
	info, err := job.FetchInfo(c)
	if err != nil {
		return nil, toStatus(err)
	}
	return &api.JobState{Config: cfg, Info: info}, nil
}
