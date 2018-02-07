// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/common"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
)

// Internal returns non-ACLed implementation of cas.StorageService.
//
// It can be used internally by the backend. Assumes ACL checks are already
// done.
func Internal() api.StorageServer {
	return &storageImpl{
		settings:     settings.Get,
		getSignedURL: getSignedURL,
	}
}

// storageImpl implements api.StorageServer.
//
// Doesn't do any ACL checks.
type storageImpl struct {
	// Mocking points for tests. See Internal() for real implementations.
	settings     func(c context.Context) (*settings.Settings, error)
	getSignedURL func(c context.Context, gsPath, filename string, signer signerFactory, gs gs.GoogleStorage) (string, error)
}

// GetObjectURL implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) GetObjectURL(c context.Context, r *api.GetObjectURLRequest) (resp *api.ObjectURL, err error) {
	defer func() { err = common.GRPCifyAndLogErr(c, err) }()

	if err := ValidateObjectRef(r.Object); err != nil {
		return nil, errors.Annotate(err, "bad 'object' field").Err()
	}

	// Lite validation for Content-Disposition header. As long as the filename
	// doesn't have '"' or '\n', we are constructing a valid header. Let the
	// browser do the rest of the validation however it likes.
	if strings.ContainsAny(r.DownloadFilename, "\"\r\n") {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'download_filename' field, contains one of %q", "\"\r\n")
	}

	cfg, err := s.settings(c)
	if err != nil {
		return nil, err
	}

	sigFactory := defaultSigner
	if cfg.SignAs != "" {
		sigFactory = func(c context.Context) (*signer, error) {
			return iamSigner(c, cfg.SignAs)
		}
	}

	url, err := s.getSignedURL(c, cfg.ObjectPath(r.Object), r.DownloadFilename, sigFactory, gs.Get(c))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get signed URL").Err()
	}
	return &api.ObjectURL{SignedUrl: url}, nil
}
