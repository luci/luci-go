// Copyright 2023 The LUCI Authors.
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

// Package remote implements backends for config client which will make calls
// to the real Config Service.
package remote

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"sort"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/srvhttp"

	"go.chromium.org/luci/config"
	pb "go.chromium.org/luci/config_service/proto"
)

// ClientFactory returns HTTP client to use (given a context).
//
// See 'New' for more details.
type ClientFactory func(context.Context) (*http.Client, error)

// retryPolicy is the default grpc retry policy for this Luci-config client.
const retryPolicy = `{
	"methodConfig": [{
		"name": [{ "service": "config.service.v2.Configs" }],
		"timeout": "60s",
		"retryPolicy": {
		  "maxAttempts": 5,
		  "initialBackoff": "1s",
		  "maxBackoff": "10s",
		  "backoffMultiplier": 1.5,
		  "retryableStatusCodes": ["UNAVAILABLE", "INTERNAL", "UNKNOWN"]
		}
	}]
}`

const (
	// defaultUserAgent is the default user-agent header value to use.
	defaultUserAgent = "Config Go Client 1.0"
)

type Options struct {
	// Host is the hostname of a LUCI Config service.
	Host string

	// Creds is the credential to use when creating the grpc connection.
	Creds credentials.PerRPCCredentials

	// UserAgent is the optional additional User-Agent fragment which will be
	// appended to gRPC calls
	//
	// If empty, defaultUserAgent is used.
	UserAgent string

	// DialOpts are the options to use to dial.
	//
	// If nil, DefaultDialOptions() are used
	DialOpts []grpc.DialOption
}

// DefaultDialOptions returns default grpc dial options.
func DefaultDialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{}),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultServiceConfig(retryPolicy),
		// LUCI Config can return gzip-compressed msg. But the grpc client doesn't
		// provide a way to check the pure compressed response size. It also checks
		// size after decompression. It's hard to set a fixed size. And for very
		// large size config, LUCI Config already uses GCS to pass the file. So it's
		// fine to not limit the received msg size.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32)),
	}
}

// New returns an implementation of the config Interface which talks to the
// real LUCI config service.
func New(ctx context.Context, opts Options) (config.Interface, error) {
	if opts.Host == "" {
		return nil, errors.New("host is not specified")
	}

	dialOpts := opts.DialOpts
	if dialOpts == nil {
		dialOpts = DefaultDialOptions()
	}
	if opts.Creds != nil {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(opts.Creds))
	}
	if opts.UserAgent != "" {
		dialOpts = append(dialOpts, grpc.WithUserAgent(opts.UserAgent))
	} else {
		dialOpts = append(dialOpts, grpc.WithUserAgent(defaultUserAgent))
	}

	conn, err := grpc.NewClient(opts.Host+":443", dialOpts...)
	if err != nil {
		return nil, errors.Fmt("cannot dial to %s: %w", opts.Host, err)
	}

	return &remoteImpl{
		conn:       conn,
		grpcClient: pb.NewConfigsClient(conn),
		httpClient: srvhttp.DefaultClient(ctx),
	}, nil
}

var _ config.Interface = &remoteImpl{}

// remoteImpl implements config.Interface and will make gRPC calls to LUCI
// Config gRPC service.
type remoteImpl struct {
	conn       *grpc.ClientConn
	grpcClient pb.ConfigsClient
	// A http client with no additional authentication. Only used for downloading from signed urls.
	httpClient *http.Client
}

func (r *remoteImpl) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	if err := r.checkInitialized(); err != nil {
		return nil, err
	}
	req := &pb.GetConfigRequest{
		ConfigSet: string(configSet),
		Path:      path,
	}
	if metaOnly {
		req.Fields = &fieldmaskpb.FieldMask{
			Paths: []string{"config_set", "path", "content_sha256", "revision", "url"},
		}
	}

	res, err := r.grpcClient.GetConfig(ctx, req, grpc.UseCompressor(gzip.Name))
	if err != nil {
		return nil, wrapGrpcErr(err)
	}

	cfg := toConfig(res)
	if res.GetSignedUrl() != "" {
		content, err := config.DownloadConfigFromSignedURL(ctx, r.httpClient, res.GetSignedUrl())
		if err != nil {
			return nil, transient.Tag.Apply(err)
		}
		cfg.Content = string(content)
	}

	return cfg, nil
}

func (r *remoteImpl) GetConfigs(ctx context.Context, cfgSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	if err := r.checkInitialized(); err != nil {
		return nil, err
	}

	// Fetch the list of files in the config set together with their hashes.
	confSetPb, err := r.grpcClient.GetConfigSet(ctx, &pb.GetConfigSetRequest{
		ConfigSet: string(cfgSet),
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"configs"},
		},
	})
	if err != nil {
		return nil, wrapGrpcErr(err)
	}

	// An edge case. This should be impossible in practice.
	if len(confSetPb.Configs) == 0 {
		return nil, nil
	}

	// Assert all returned files are from the same revision. They should be.
	rev := confSetPb.Configs[0].Revision
	for _, cfg := range confSetPb.Configs {
		if cfg.Revision != rev {
			return nil, errors.Fmt("internal error: the reply contains files from revisions %q and %q", cfg.Revision, rev)
		}
	}

	// Filter the file list through the callback.
	var filtered []*pb.Config
	if filter != nil {
		filtered = confSetPb.Configs[:0]
		for _, cfg := range confSetPb.Configs {
			if filter(cfg.Path) {
				filtered = append(filtered, cfg)
			}
		}
	} else {
		filtered = confSetPb.Configs
	}

	// If the caller only cares about metadata, we are done.
	if metaOnly {
		out := make(map[string]config.Config, len(filtered))
		for _, cfg := range filtered {
			cfg.Content = nil // in case the server decides to return something
			out[cfg.Path] = *toConfig(cfg)
		}
		return out, nil
	}

	// Fetch all files in parallel using their SHA256 as the key.
	out := make(map[string]config.Config, len(filtered))
	var m sync.Mutex
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for _, cfg := range filtered {
		eg.Go(func() error {
			body, err := r.grpcClient.GetConfig(ectx, &pb.GetConfigRequest{
				ConfigSet:     string(cfgSet),
				ContentSha256: cfg.ContentSha256,
			}, grpc.UseCompressor(gzip.Name))
			if err != nil {
				err = wrapGrpcErr(err)
				// Do not return ErrNoConfig if an individual file is missing. First of
				// all, it should never happen. If it does happen for some reason, we
				// must not return ErrNoConfig anyway, because it will be interpreted
				// as if the config set is gone, which will be incorrect.
				if err == config.ErrNoConfig {
					return errors.Fmt("internal error: config %q at SHA256 %q is unexpectedly gone", cfg.Path, cfg.ContentSha256)
				}
				return errors.Fmt("fetching %q at SHA256 %q: %w", cfg.Path, cfg.ContentSha256, err)
			}

			// Ignore all metadata from `body`. It may be pointing to some other
			// file or revision that happened to have the exact same SHA256 as the one
			// we are requesting. We only care about the content.
			resolved := toConfig(cfg)
			if url := body.GetSignedUrl(); url != "" {
				content, err := config.DownloadConfigFromSignedURL(ectx, r.httpClient, url)
				if err != nil {
					return transient.Tag.Apply(errors.Fmt("fetching %q from signed URL: %w", cfg.Path, err))
				}
				resolved.Content = string(content)
			} else {
				resolved.Content = string(body.GetRawContent())
			}

			m.Lock()
			out[resolved.Path] = *resolved
			m.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *remoteImpl) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	if err := r.checkInitialized(); err != nil {
		return nil, err
	}
	req := &pb.GetProjectConfigsRequest{Path: path}
	if metaOnly {
		req.Fields = &fieldmaskpb.FieldMask{
			Paths: []string{"config_set", "path", "content_sha256", "revision", "url"},
		}
	}

	// This rpc response is usually larger than others. So instruct the Server to
	// return a compressed response to allow data transfer faster.
	res, err := r.grpcClient.GetProjectConfigs(ctx, req, grpc.UseCompressor(gzip.Name))
	if err != nil {
		return nil, wrapGrpcErr(err)
	}

	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	configs := make([]config.Config, len(res.Configs))
	for i, cfg := range res.Configs {
		configs[i] = *toConfig(cfg)
		if cfg.GetSignedUrl() != "" {
			i := i
			signedURL := cfg.GetSignedUrl()
			eg.Go(func() error {
				content, err := config.DownloadConfigFromSignedURL(ectx, r.httpClient, signedURL)
				if err != nil {
					return transient.Tag.Apply(errors.Fmt("for file(%s) in config_set(%s): %w", configs[i].Path, configs[i].ConfigSet, err))
				}
				configs[i].Content = string(content)
				return nil
			})
		}
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *remoteImpl) GetProjects(ctx context.Context) ([]config.Project, error) {
	if err := r.checkInitialized(); err != nil {
		return nil, err
	}

	res, err := r.grpcClient.ListConfigSets(ctx, &pb.ListConfigSetsRequest{Domain: pb.ListConfigSetsRequest_PROJECT})
	if err != nil {
		return nil, wrapGrpcErr(err)
	}

	projects := make([]config.Project, len(res.ConfigSets))
	for i, cs := range res.ConfigSets {
		projectID := config.Set(cs.Name).Project()
		parsedURL, err := url.Parse(cs.Url)
		if err != nil {
			return nil, errors.Fmt("failed to parse repo url %s in project %s: %w", cs.Url, projectID, err)
		}
		projects[i] = config.Project{
			ID:       projectID,
			Name:     projectID,
			RepoURL:  parsedURL,
			RepoType: config.GitilesRepo,
		}
	}

	return projects, nil
}

func (r *remoteImpl) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	if err := r.checkInitialized(); err != nil {
		return nil, err
	}

	res, err := r.grpcClient.GetConfigSet(ctx, &pb.GetConfigSetRequest{
		ConfigSet: string(configSet),
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"configs"},
		},
	})
	if err != nil {
		return nil, wrapGrpcErr(err)
	}

	paths := make([]string, len(res.Configs))
	for i, cfg := range res.Configs {
		paths[i] = cfg.Path
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *remoteImpl) Close() error {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.Close()
}

func (r *remoteImpl) checkInitialized() error {
	if r == nil || r.grpcClient == nil || r.httpClient == nil {
		return errors.New("The Luci-config client is not initialized")
	}
	return nil
}

func wrapGrpcErr(err error) error {
	switch code := grpcutil.Code(err); {
	case code == codes.NotFound:
		return config.ErrNoConfig
	case grpcutil.IsTransientCode(code):
		return transient.Tag.Apply(err)
	default:
		return err
	}
}

func toConfig(configPb *pb.Config) *config.Config {
	return &config.Config{
		Meta: config.Meta{
			ConfigSet:   config.Set(configPb.ConfigSet),
			Path:        configPb.Path,
			ContentHash: configPb.ContentSha256,
			Revision:    configPb.Revision,
			ViewURL:     configPb.Url,
		},
		Content: string(configPb.GetRawContent()),
	}
}
