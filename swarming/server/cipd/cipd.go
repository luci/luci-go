// Copyright 2024 The LUCI Authors.
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

// Package cipd is a CIPD client used by the Swarming server.
package cipd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	cipdgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/grpcpb"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"
)

// Client can talk to the CIPD server.
type Client struct {
	m      sync.RWMutex
	perSrv map[string]cipdgrpcpb.RepositoryClient
}

// ResolveVersion asks the CIPD server to resolve a version label into a
// concrete CIPD instance ID.
//
// This also indirectly checks if such package version exists at all.
//
// Returns gRPC errors.
func (c *Client) ResolveVersion(ctx context.Context, server, cipdpkg, version string) (string, error) {
	client, err := c.client(ctx, server)
	if err != nil {
		return "", err
	}

	resp, err := client.ResolveVersion(ctx,
		&cipdpb.ResolveVersionRequest{
			Package: cipdpkg,
			Version: version,
		},
	)
	if err != nil {
		return "", err
	}
	resolved := common.ObjectRefToInstanceID(resp.Instance)

	// If the initially requested version is already an instance ID (i.e. the
	// expected hash of the package content), the server should return it as is.
	// This check below allows to avoid trusting the CIPD server when using
	// instance IDs as versions. If the server lies, we'll just error out. We
	// still need to call ResolveVersion to check if such instance exists at all
	// though.
	if common.ValidateInstanceID(version, common.AnyHash) == nil && resolved != version {
		return "", status.Errorf(codes.Internal, "CIPD server resolved instance ID %q to %q which is wrong", version, resolved)
	}

	return resolved, nil
}

// FetchInstance fetches contents of a package given via its instance ID.
//
// The returned package instance must be eventually closed. Do not use with big
// packages, it will OOM.
//
// Returns gRPC errors.
func (c *Client) FetchInstance(ctx context.Context, server, cipdpkg, iid string) (pkg.Instance, error) {
	if err := common.ValidateInstanceID(iid, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}
	client, err := c.client(ctx, server)
	if err != nil {
		return nil, err
	}

	// Get a signed download URL.
	objectURL, err := client.GetInstanceURL(ctx, &cipdpb.GetInstanceURLRequest{
		Package:  cipdpkg,
		Instance: common.InstanceIDToObjectRef(iid),
	})
	if err != nil {
		return nil, err
	}

	// Fetch the file.
	req, err := http.NewRequestWithContext(ctx, "GET", objectURL.SignedUrl, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "bad instance URL: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "IO error initiating the fetch: %s", err)
	}
	blob, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "IO error fetching the package: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		logging.Errorf(ctx, "Unexpected HTTP status %d:\n%s", resp.StatusCode, blob)
		return nil, status.Errorf(codes.Internal, "unexpected HTTP status %d when fetching the package", resp.StatusCode)
	}

	// Verify the hash and read the package content.
	inst, err := reader.OpenInstance(ctx, pkg.NewBytesSource(blob), reader.OpenInstanceOpts{
		VerificationMode: reader.VerifyHash,
		InstanceID:       iid,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "bad package body: %s", err)
	}
	return inst, nil
}

// client returns a CIPD client connected to a backend at the given address.
func (c *Client) client(ctx context.Context, server string) (cipdgrpcpb.RepositoryClient, error) {
	parsed, err := url.Parse(server)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "not a valid CIPD URL %q", server)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, status.Errorf(codes.InvalidArgument, "expecting a root URL, not %q", server)
	}
	server = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	c.m.RLock()
	client := c.perSrv[server]
	c.m.RUnlock()
	if client != nil {
		return client, nil
	}

	c.m.Lock()
	defer c.m.Unlock()

	if client = c.perSrv[server]; client != nil {
		return client, nil
	}

	logging.Infof(ctx, "Connecting to CIPD at %s", server)
	client = cipdgrpcpb.NewRepositoryClient(&prpc.Client{
		C:    http.DefaultClient, // no authentication, bot packages are public
		Host: parsed.Host,
		Options: &prpc.Options{
			Insecure: parsed.Scheme == "http", // for testing with the local server
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 10,
					},
				}
			},
		},
	})

	if c.perSrv == nil {
		c.perSrv = make(map[string]cipdgrpcpb.RepositoryClient, 1)
	}
	c.perSrv[server] = client

	return client, nil
}
