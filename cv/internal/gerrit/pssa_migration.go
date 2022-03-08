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

package gerrit

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	luciauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
)

// projectsToMigrate are all projects that haven't opt-in to using Project
// Scoped Service Account as of Mar, 2022.
var projectsToMigrate = stringset.NewFromSlice(
	"art",
	"cast-chromecast-internal",
	"chromeos",
	"dart",
	"fuchsia",
	"fuchsia-cobalt",
	"fuchsia-fuchsia",
	"fuchsia-infra-recipes",
	"fuchsia-tools",
	"gazoo",
	"gn",
	"goma-server-internal",
	"r8",
	"shaka-packager",
	"skia",
	"skia-internal",
	"skia-internal-test",
	"skia-lottie-ci",
	"skia-skcms",
	"skiabot-test",
	"skiabuildbot",
	"turquoise",
	"turquoise-infra-gce-mediator",
	"turquoise-infra-grift",
	"v8-restricted",
	"widevine-cdm",
)

// PSSAMigrationFactory returns a factory used for Project Scoped Service
// Account(PSSA) migration.
//
// CV must use project-scoped credentials, but not every project has configured
// PSSA. The alternative and legacy authentication is based on per GerritHost
// auth tokens from ~/.netrc shared by all LUCI projects.
//
// For smooth migration, the client will use pssa client to call Gerrit first.
// If the call fails with empty Project token (PSSA not configured) or
// permission denied (PSSA is configured, but is likely not granted with the
// right access), it fallbacks to use legacy netrc token. In this way,
// we can enable the PSSA for project then fix any permission issue behind
// scene without breaking any users.
func PSSAMigrationFactory(f Factory) Factory {
	return pssaMigrationFactory{
		passClientfactory: f,
		// CV supports <20 legacy hosts. New ones shouldn't be added.
		legacyTokenCache: lru.New(20),
	}
}

type pssaMigrationFactory struct {
	// passClientfactory is the factory to make Gerrit client that use Project
	// Scoped Service Account to authenticate.
	passClientfactory Factory
	// legacyTokenCache caches legacy tokens per gerritHost.
	legacyTokenCache *lru.Cache
	baseTransport    http.RoundTripper
}

func (p pssaMigrationFactory) MakeMirrorIterator(ctx context.Context) *MirrorIterator {
	return p.passClientfactory.MakeMirrorIterator(ctx)
}

// MakeClient implements Factory.
func (p pssaMigrationFactory) MakeClient(ctx context.Context, gerritHost string, luciProject string) (Client, error) {
	pssaClient, err := p.passClientfactory.MakeClient(ctx, gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	var legacyClient gerritpb.GerritClient
	if projectsToMigrate.Has(luciProject) {
		base := p.baseTransport
		if base == nil {
			base, err = auth.GetRPCTransport(ctx, auth.NoAuth)
			if err != nil {
				return nil, err
			}
		}
		legacyClient, err = gerrit.NewRESTClient(
			&http.Client{
				Transport: p.legacyTransport(base, gerritHost, luciProject),
			}, gerritHost, true)
		if err != nil {
			return nil, err
		}
	}

	return pssaMigrationClient{
		host:         gerritHost,
		pssaClient:   pssaClient,
		legacyClient: legacyClient,
	}, nil
}

func (p *pssaMigrationFactory) legacyTransport(base http.RoundTripper, gerritHost, luciProject string) http.RoundTripper {
	return luciauth.NewModifyingTransport(base, func(req *http.Request) error {
		ctx := req.Context()
		value, err := p.legacyTokenCache.GetOrCreate(ctx, gerritHost, func() (value interface{}, ttl time.Duration, err error) {
			nt := netrcToken{GerritHost: gerritHost}
			switch err = datastore.Get(ctx, &nt); {
			case err == datastore.ErrNoSuchEntity:
				// While not expected in practice, speed up rollout of a fix by caching
				// for a short time only.
				ttl = 1 * time.Minute
				value = ""
				err = nil
			case err != nil:
				err = errors.Annotate(err, "failed to get legacy creds").Tag(transient.Tag).Err()
			default:
				value = nt.AccessToken
				ttl = 10 * time.Minute
			}
			return
		})

		switch {
		case err != nil:
			return err
		case value.(string) == "":
			return errors.Reason("No legacy credentials for host %q", gerritHost).Err()
		}
		tok := &oauth2.Token{
			AccessToken: base64.StdEncoding.EncodeToString([]byte(value.(string))),
			TokenType:   "Basic",
		}
		req.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
		return nil
	})
}

type pssaMigrationClient struct {
	host         string
	pssaClient   Client
	legacyClient Client
}

func (p pssaMigrationClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	switch res, err := p.pssaClient.ListChanges(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.ListChanges(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: ListChange, request: %s", p.host, in)
		return p.legacyClient.ListChanges(ctx, in, opts...)
	default:
		return res, err
	}
}

func (p pssaMigrationClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	switch res, err := p.pssaClient.GetChange(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.GetChange(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: GetChange, request: %s", p.host, in)
		return p.legacyClient.GetChange(ctx, in, opts...)
	default:
		return res, err
	}
}

func (p pssaMigrationClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	switch res, err := p.pssaClient.GetRelatedChanges(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.GetRelatedChanges(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: GetRelatedChanges, request: %s", p.host, in)
		return p.legacyClient.GetRelatedChanges(ctx, in, opts...)
	default:
		return res, err
	}
}

func (p pssaMigrationClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	switch res, err := p.pssaClient.ListFiles(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.ListFiles(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: ListFiles, request: %s", p.host, in)
		return p.legacyClient.ListFiles(ctx, in, opts...)
	default:
		return res, err
	}
}

func (p pssaMigrationClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	switch res, err := p.pssaClient.SetReview(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.SetReview(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: SetReview, request: %s", p.host, in)
		return p.legacyClient.SetReview(ctx, in, opts...)
	default:
		return res, err
	}
}

func (p pssaMigrationClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error) {
	switch res, err := p.pssaClient.SubmitRevision(ctx, in, opts...); {
	case p.legacyClient == nil || err == nil:
		return res, err
	case strings.Contains(err.Error(), errEmptyProjectToken.Error()):
		return p.legacyClient.SubmitRevision(ctx, in, opts...)
	case grpcutil.Code(err) == codes.PermissionDenied:
		logging.Warningf(ctx, "crbug.com/824492: get permission denied when using "+
			"pssa to call Gerrit, fallback to use legacy netrc token. "+
			"host: %s, method: SubmitRevision, request: %s", p.host, in)
		return p.legacyClient.SubmitRevision(ctx, in, opts...)
	default:
		return res, err
	}
}

// netrcToken stores ~/.netrc access tokens of CQDaemon.
type netrcToken struct {
	GerritHost  string `gae:"$id"`
	AccessToken string `gae:",noindex"`
}

// SaveLegacyNetrcToken creates or updates legacy netrc token.
func SaveLegacyNetrcToken(ctx context.Context, host, token string) error {
	err := datastore.Put(ctx, &netrcToken{host, token})
	return errors.Annotate(err, "failed to save legacy netrc token").Err()
}
