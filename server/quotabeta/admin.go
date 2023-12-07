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

package quota

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"text/template"

	"github.com/gomodule/redigo/redis"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/runtime/protoiface"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/server/auth"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
)

// getEntry is a *template.Template for a Lua script which gets the given quota
// entry from Redis. Should be used after a single updateEntry.
//
// Template variables:
// Var: Name of a Lua variable holding the quota entry in memory.
var getEntry = template.Must(template.New("getEntry").Parse(`
	return {{.Var}}["resources"]
`))

// Ensure quotaAdmin implements QuotaAdminServer at compile-time.
var _ pb.QuotaAdminServer = &quotaAdmin{}

// quotaAdmin implements pb.QuotaAdminServer.
type quotaAdmin struct {
}

// Get returns the available resources for the given policy.
func (*quotaAdmin) Get(ctx context.Context, req *pb.GetRequest) (*pb.QuotaEntry, error) {
	if req.GetPolicy() == "" {
		return nil, appstatus.Errorf(codes.InvalidArgument, "policy is required")
	}

	now := clock.Now(ctx).Unix()
	cfg := getInterface(ctx)
	rsp := &pb.QuotaEntry{}

	rsp.Name = req.Policy
	if strings.Contains(req.Policy, "${user}") {
		if req.User == "" {
			return nil, appstatus.BadRequest(errors.New("user not specified"))
		}
		rsp.Name = strings.ReplaceAll(rsp.Name, "${user}", req.User)
	}
	rsp.DbName = fmt.Sprintf("entry:%x", sha256.Sum256([]byte(rsp.Name)))
	def, err := cfg.Get(ctx, req.Policy)
	switch {
	case err == quotaconfig.ErrNotFound:
		return nil, appstatus.Errorf(codes.NotFound, "policy %q (db name: %s) not found", rsp.Name, rsp.DbName)
	case err != nil:
		return nil, errors.Annotate(err, "fetching config").Err()
	}

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "establishing connection").Err()
	}
	defer conn.Close()

	s := bytes.NewBufferString("local entry = {}\n")
	if err := updateEntry.Execute(s, map[string]any{
		"Var":           "entry",
		"Name":          rsp.DbName,
		"Default":       def.Resources,
		"Now":           now,
		"Replenishment": def.Replenishment,
		"Amount":        0,
	}); err != nil {
		return nil, errors.Annotate(err, "rendering template %q", updateEntry.Name()).Err()
	}
	if err := getEntry.Execute(s, map[string]any{
		"Var": "entry",
	}); err != nil {
		return nil, errors.Annotate(err, "rendering template %q", setEntry.Name()).Err()
	}

	val, err := redis.NewScript(0, s.String()).Do(conn)
	if err != nil {
		return nil, err
	}
	res, ok := val.(int64)
	if !ok {
		return nil, errors.Annotate(err, "expected int64 not %T", val).Err()
	}
	rsp.Resources = res
	return rsp, nil
}

// overwriteEntry is a *template.Template for a Lua script which sets the given
// quota entry in Redis, overwriting any existing entry.
//
// Template variables:
// Name: Name of the quota entry to update.
// Resources: Number of resources to set.
// Now: Update time in seconds since epoch.
var overwriteEntry = template.Must(template.New("getEntry").Parse(`
	return redis.call("HMSET", "{{.Name}}", "resources", {{.Resources}}, "updated", {{.Now}})
`))

// Set updates the available resources for the given policy.
func (*quotaAdmin) Set(ctx context.Context, req *pb.SetRequest) (*pb.QuotaEntry, error) {
	switch {
	case req.GetPolicy() == "":
		return nil, appstatus.Errorf(codes.InvalidArgument, "policy is required")
	case req.Resources < 0:
		return nil, appstatus.Errorf(codes.InvalidArgument, "resources must not be negative")
	}

	now := clock.Now(ctx).Unix()
	cfg := getInterface(ctx)
	rsp := &pb.QuotaEntry{}

	rsp.Name = req.Policy
	if strings.Contains(req.Policy, "${user}") {
		if req.User == "" {
			return nil, appstatus.BadRequest(errors.New("user not specified"))
		}
		rsp.Name = strings.ReplaceAll(rsp.Name, "${user}", req.User)
	}
	rsp.DbName = fmt.Sprintf("entry:%x", sha256.Sum256([]byte(rsp.Name)))
	def, err := cfg.Get(ctx, req.Policy)
	switch {
	case err == quotaconfig.ErrNotFound:
		return nil, appstatus.Errorf(codes.NotFound, "policy %q (db name: %s) not found", rsp.Name, rsp.DbName)
	case err != nil:
		return nil, errors.Annotate(err, "fetching config").Err()
	}
	rsp.Resources = req.Resources
	if rsp.Resources > def.Resources {
		rsp.Resources = def.Resources
	}

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "establishing connection").Err()
	}
	defer conn.Close()

	s := bytes.NewBufferString("")
	if err := overwriteEntry.Execute(s, map[string]any{
		"Name":      rsp.DbName,
		"Resources": rsp.Resources,
		"Now":       now,
	}); err != nil {
		return nil, errors.Annotate(err, "rendering template %q", overwriteEntry.Name()).Err()
	}

	if _, err := redis.NewScript(0, s.String()).Do(conn); err != nil {
		return nil, err
	}
	return rsp, nil
}

// NewQuotaAdminServer returns a pb.QuotaAdminServer with ACLs limited to the
// given groups. Readers have access to Get.
// TODO(crbug/1280055): Add more admin methods, detail access here.
func NewQuotaAdminServer(readerGroup, writerGroup string) pb.QuotaAdminServer {
	writers := []string{writerGroup}
	readers := append(writers, readerGroup)
	return &pb.DecoratedQuotaAdmin{
		// Prelude restricts access to the given groups.
		Prelude: func(ctx context.Context, methodName string, _ protoiface.MessageV1) (context.Context, error) {
			groups := writers
			if methodName == "Get" {
				groups = readers
			}
			switch is, err := auth.IsMember(ctx, groups...); {
			case err != nil:
				return ctx, errors.Annotate(err, "auth.IsMember").Err()
			case is:
				logging.Debugf(ctx, "%s called %q", auth.CurrentIdentity(ctx), methodName)
				return ctx, nil
			default:
				return ctx, appstatus.Errorf(codes.PermissionDenied, "unauthorized user %s", auth.CurrentIdentity(ctx))
			}
		},

		Service: &quotaAdmin{},

		// Postlude logs non-GRPC errors, and returns them as gRPC internal errors.
		Postlude: func(ctx context.Context, _ string, _ protoiface.MessageV1, err error) error {
			return appstatus.GRPCifyAndLog(ctx, err)
		},
	}
}
