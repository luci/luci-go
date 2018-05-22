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

package cipd

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

var errNoV2Impl = errors.New("this call is not yet implemented in v2 protocol")

// prpcRemoteImpl implements v1 'remote' interface using v2 protocol.
//
// It exists temporarily during the transition from v1 to v2 protocol. Once
// v2 is fully implemented and becomes the default, v1 implementation of the
// 'remote' interface will be deleted, and the interface itself will be adjusted
// to  match v2 protocol better (or deleted completely, since pRPC-level
// interface is good enough on its own).
type prpcRemoteImpl struct {
	serviceURL string
	userAgent  string
	client     *http.Client

	cas  api.StorageClient
	repo api.RepositoryClient
}

func (r *prpcRemoteImpl) init() error {
	// Note: serviceURL is ALWAYS "http(s)://<host>" here, as setup by NewClient.
	parsed, err := url.Parse(r.serviceURL)
	if err != nil {
		panic(err)
	}
	prpcC := &prpc.Client{
		C:    r.client,
		Host: parsed.Host,
		Options: &prpc.Options{
			UserAgent: r.userAgent,
			Insecure:  parsed.Scheme == "http", // for testing with local dev server
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 5,
					},
				}
			},
		},
	}

	r.cas = api.NewStoragePRPCClient(prpcC)
	r.repo = api.NewRepositoryPRPCClient(prpcC)
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// ACLs.

func (r *prpcRemoteImpl) fetchACL(ctx context.Context, packagePath string) ([]PackageACL, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) modifyACL(ctx context.Context, packagePath string, changes []PackageACLChange) error {
	return errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Upload.

func (r *prpcRemoteImpl) initiateUpload(ctx context.Context, sha1 string) (*UploadSession, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) finalizeUpload(ctx context.Context, sessionID string) (bool, error) {
	return false, errNoV2Impl
}

func (r *prpcRemoteImpl) registerInstance(ctx context.Context, pin common.Pin) (*registerInstanceResponse, error) {
	return nil, errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Fetching.

func (r *prpcRemoteImpl) resolveVersion(ctx context.Context, packageName, version string) (common.Pin, error) {
	return common.Pin{}, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchPackage(ctx context.Context, packageName string, withRefs bool) (*fetchPackageResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchInstance(ctx context.Context, pin common.Pin) (*fetchInstanceResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchClientBinaryInfo(ctx context.Context, pin common.Pin) (*fetchClientBinaryInfoResponse, error) {
	return nil, errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Refs and tags.

func (r *prpcRemoteImpl) setRef(ctx context.Context, ref string, pin common.Pin) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) attachTags(ctx context.Context, pin common.Pin, tags []string) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) fetchTags(ctx context.Context, pin common.Pin, tags []string) ([]TagInfo, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchRefs(ctx context.Context, pin common.Pin, refs []string) ([]RefInfo, error) {
	return nil, errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Misc.

func (r *prpcRemoteImpl) listPackages(ctx context.Context, path string, recursive, showHidden bool) ([]string, []string, error) {
	return nil, nil, errNoV2Impl
}

func (r *prpcRemoteImpl) searchInstances(ctx context.Context, tag, packageName string) (common.PinSlice, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) listInstances(ctx context.Context, packageName string, limit int, cursor string) (*listInstancesResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) deletePackage(ctx context.Context, packageName string) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) incrementCounter(ctx context.Context, pin common.Pin, counter string, delta int) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) readCounter(ctx context.Context, pin common.Pin, counter string) (Counter, error) {
	return Counter{}, errNoV2Impl
}
