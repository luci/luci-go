// Copyright 2020 The LUCI Authors.
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

// Package casclient provides remote-apis-sdks client with luci integration.
package casclient

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/cas"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/contextmd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// AddrProd is the PROD CAS service address.
const AddrProd = "remotebuildexecution.googleapis.com:443"

// New returns luci auth configured Client for RBE-CAS.
func New(ctx context.Context, addr string, instance string, opts auth.Options, readOnly bool) (*cas.Client, error) {
	var dialParams client.DialParams
	useLocal, err := isLocalAddr(addr)
	if err != nil {
		return nil, errors.Annotate(err, "invalid addr").Err()
	}
	if useLocal {
		// Connect to local fake CAS server.
		// See also go.chromium.org/luci/tools/cmd/fakecas
		if instance != "" {
			return nil, errors.Reason("do not specify instance with local address").Err()
		}
		instance = "instance"
		dialParams = client.DialParams{
			Service:    addr,
			NoSecurity: true,
		}
	} else {
		creds, err := perRPCCreds(ctx, instance, opts, readOnly)
		if err != nil {
			return nil, err
		}

		dialParams = client.DialParams{
			Service:              addr,
			UseExternalAuthToken: true,
			ExternalPerRPCCreds:  &client.PerRPCCreds{Creds: creds},
		}
	}

	grpcOpts, _, err := client.OptsFromParams(ctx, dialParams)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get grpc opts").Err()
	}
	conn, err := grpc.Dial(dialParams.Service, grpcOpts...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to dial RBE").Err()
	}

	cl, err := cas.NewClientWithConfig(ctx, conn, instance, DefaultConfig())
	if err != nil {
		return nil, errors.Annotate(err, "failed to create client").Err()
	}
	return cl, nil
}

// DefaultConfig returns default CAS client configuration.
func DefaultConfig() cas.ClientConfig {
	cfg := cas.DefaultClientConfig()
	cfg.CompressedBytestreamThreshold = 0 // compress always

	// The default 1 min timeout seems slow for some kinds of uploads, see also
	// Options().
	cfg.ByteStreamWrite.Timeout = 2 * time.Minute

	// Do not read file less than 10MiB twice.
	cfg.SmallFileThreshold = 10 * 1024 * 1024

	return cfg
}

func perRPCCreds(ctx context.Context, instance string, opts auth.Options, readOnly bool) (credentials.PerRPCCredentials, error) {
	project := strings.Split(instance, "/")[1]
	var role string
	if readOnly {
		role = "cas-read-only"
	} else {
		role = "cas-read-write"
	}

	// Construct auth.Options.
	opts.ActAsServiceAccount = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", role, project)
	opts.ActViaLUCIRealm = fmt.Sprintf("@internal:%s/%s", project, role)
	opts.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}

	if strings.HasSuffix(project, "-dev") || strings.HasSuffix(project, "-staging") {
		// use dev token server for dev/staging projects.
		opts.TokenServerHost = chromeinfra.TokenServerDevHost
	}

	creds, err := auth.NewAuthenticator(ctx, auth.SilentLogin, opts).PerRPCCredentials()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
	}
	return creds, nil
}

// NewLegacy returns luci auth configured legacy Client for RBE.
// In general, NewClient is preferred.
// TODO(crbug.com/1225524): remove this.
func NewLegacy(ctx context.Context, addr string, instance string, opts auth.Options, readOnly bool) (*client.Client, error) {
	useLocal, err := isLocalAddr(addr)
	if err != nil {
		return nil, errors.Annotate(err, "invalid addr").Err()
	}
	if useLocal {
		// Connect to local fake CAS server.
		// See also go.chromium.org/luci/tools/cmd/fakecas
		if instance != "" {
			logging.Warningf(ctx, "instance %q is given, but will be ignored.", instance)
		}
		dialParams := client.DialParams{
			Service:    addr,
			NoSecurity: true,
		}
		cl, err := client.NewClient(ctx, "instance", dialParams)
		if err != nil {
			return nil, errors.Annotate(err, "failed to create client").Err()
		}
		return cl, nil
	}

	creds, err := perRPCCreds(ctx, instance, opts, readOnly)
	if err != nil {
		return nil, err
	}
	dialParams := client.DialParams{
		Service:              "remotebuildexecution.googleapis.com:443",
		UseExternalAuthToken: true,
		ExternalPerRPCCreds:  &client.PerRPCCreds{Creds: creds},
	}

	cl, err := client.NewClient(ctx, instance, dialParams, Options()...)
	if err != nil {
		logging.Errorf(ctx, "failed to create casclient: %+v", err)
		return nil, errors.Annotate(err, "failed to create client").Err()
	}
	return cl, nil
}

// Options returns CAS client options.
func Options() []client.Opt {
	casConcurrency := runtime.NumCPU() * 2
	if runtime.GOOS == "windows" {
		// This is for better file write performance on Windows (http://b/171672371#comment6).
		casConcurrency = runtime.NumCPU()
	}

	rpcTimeouts := make(client.RPCTimeouts)
	for k, v := range client.DefaultRPCTimeouts {
		rpcTimeouts[k] = v
	}

	// Extend the timeout for write operations beyond the default, as writes can
	// sometimes be quite slow. This timeout only applies to writing a single
	// file chunk, so there isn't a risk of setting a timeout that's to low for
	// large files.
	rpcTimeouts["Write"] = 2 * time.Minute

	// There's suspicion GetCapabilities sometimes takes longer than default
	// 5 sec because it is the first call ever (and it needs to open the
	// connection and refresh auth tokens). Give it more time.
	rpcTimeouts["GetCapabilities"] = 30 * time.Second

	return []client.Opt{
		client.CASConcurrency(casConcurrency),
		client.UtilizeLocality(true),
		&client.TreeSymlinkOpts{
			// Symlinks will be uploaded as-is...
			Preserved: true,
			// ... and the target file included in the CAS archive...
			FollowsTarget: true,
			// ... unless the target file is outside the root directory, in
			// which case the target file will be uploaded instead of preserving
			// the symlink.
			MaterializeOutsideExecRoot: true,
		},
		rpcTimeouts,
		// Set restricted permission for written files.
		client.DirMode(0700),
		client.ExecutableMode(0700),
		client.RegularMode(0600),
		client.CompressedBytestreamThreshold(0),
	}
}

// ContextWithMetadata attaches RBE related metadata with tool name to the
// given context.
func ContextWithMetadata(ctx context.Context, toolName string) (context.Context, error) {
	ctx, err := contextmd.WithMetadata(ctx, &contextmd.Metadata{
		ToolName: toolName,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to attach metadata").Err()
	}

	m, err := contextmd.ExtractMetadata(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to extract metadata").Err()
	}

	logging.Infof(ctx, "context metadata: %#+v", *m)

	return ctx, nil
}

func isLocalAddr(addr string) (bool, error) {
	tcpaddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return false, err
	}
	if tcpaddr.IP == nil {
		return true, nil
	}
	return tcpaddr.IP.IsLoopback(), nil
}
