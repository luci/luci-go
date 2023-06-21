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

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
)

func UpdateMetadata(ctx context.Context) error {
	servicesCfg := &cfgcommonpb.ServicesCfg{}
	if err := common.LoadSelfConfig(ctx, common.ServiceRegistryFilePath, servicesCfg); err != nil {
		return fmt.Errorf("failed to load %s. Reason: %w", common.ServiceRegistryFilePath, err)
	}

	toDelete, err := computeServiceMetadataToDelete(ctx, servicesCfg)
	if err != nil {
		return err
	}

	var errs []error
	var errorsMu sync.Mutex
	perr := parallel.WorkPool(8, func(workC chan<- func() error) {
		for _, srv := range servicesCfg.GetServices() {
			if srv.GetServiceEndpoint() != "" || srv.GetMetadataUrl() != "" {
				srv := srv
				workC <- func() error {
					ctx = logging.SetField(ctx, "service", srv.GetId())
					if err := updateServiceMetadata(ctx, srv); err != nil {
						errorsMu.Lock()
						errs = append(errs, err)
						errorsMu.Unlock()
						logging.Errorf(ctx, "failed to update service metadata. Reason: %s", err)
					}
					return nil
				}
			}
		}

		if len(toDelete) > 0 {
			workC <- func() error {
				services := make([]string, len(toDelete))
				for i, key := range toDelete {
					services[i] = key.StringID()
				}
				if err := datastore.Delete(ctx, toDelete); err != nil {
					return fmt.Errorf("failed to delete ServiceMetadata for deleted service(s) [%s]: %w", strings.Join(services, ", "), err)
				}
				logging.Infof(ctx, "successfully deleted ServiceMetadata for deleted service(s): [%s]", strings.Join(services, ", "))
				return nil
			}
		}
	})

	if perr != nil {
		panic(fmt.Errorf("impossible pool error %w", perr))
	}
	return errors.Join(errs...)
}

func computeServiceMetadataToDelete(ctx context.Context, servicesCfg *cfgcommonpb.ServicesCfg) ([]*datastore.Key, error) {
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery("ServiceMetadata").KeysOnly(true), &keys); err != nil {
		return nil, fmt.Errorf("failed to query all service metadata keys: %w", err)
	}
	currentServices := stringset.New(len(servicesCfg.GetServices()))
	for _, srv := range servicesCfg.GetServices() {
		currentServices.Add(srv.GetId())
	}
	toDelete := keys[:0] // reuse the memory of `keys`
	for _, key := range keys {
		if !currentServices.Has(key.StringID()) {
			toDelete = append(toDelete, key)
		}
	}
	return toDelete, nil
}

func updateServiceMetadata(ctx context.Context, srv *cfgcommonpb.Service) error {
	eg, ectx := errgroup.WithContext(ctx)
	var metadataFromService *cfgcommonpb.ServiceMetadata
	var legacyMetadataFromService *cfgcommonpb.ServiceDynamicMetadata
	switch {
	case srv.GetServiceEndpoint() != "":
		eg.Go(func() (err error) {
			metadataFromService, err = fetchMetadata(ctx, srv.GetServiceEndpoint())
			return err
		})
	case srv.GetMetadataUrl() != "":
		eg.Go(func() (err error) {
			legacyMetadataFromService, err = fetchLegacyMetadata(ctx, srv.GetMetadataUrl(), srv.GetJwtAuth().GetAudience())
			return err
		})
	default:
		panic(errors.New("impossible; must provide either service_endpoint or metadata_url"))
	}

	metadataEntity := &model.ServiceMetadata{
		ServiceName: srv.GetId(),
	}
	eg.Go(func() error {
		switch err := datastore.Get(ectx, metadataEntity); err {
		case datastore.ErrNoSuchEntity:
			// Expect entity missing for the first time updating service metadata.
			logging.Warningf(ectx, "missing ServiceMetadata datastore entity for %q. This is common for first time updating service metadata")
			return nil
		default:
			return err
		}
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	if skipUpdate(ctx, metadataEntity, metadataFromService, legacyMetadataFromService) {
		logging.Infof(ctx, "skip updating service metadata as LUCI Config already has the latest")
		return nil
	}

	metadataEntity.Metadata = metadataFromService
	metadataEntity.LegacyMetadata = legacyMetadataFromService
	metadataEntity.UpdateTime = clock.Now(ctx).UTC()
	if err := datastore.Put(ctx, metadataEntity); err != nil {
		return err
	}
	logging.Infof(ctx, "successfully updated service metadata")
	return nil
}

func fetchMetadata(ctx context.Context, endpoint string) (*cfgcommonpb.ServiceMetadata, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport %w", err)
	}
	prpcClient := &prpc.Client{
		C:    &http.Client{Transport: tr},
		Host: endpoint,
	}
	if strings.HasPrefix(endpoint, "127.0.0.1") { // testing
		prpcClient.Options = &prpc.Options{Insecure: true}
	}
	client := cfgcommonpb.NewConsumerClient(prpcClient)
	return client.GetMetadata(ctx, &emptypb.Empty{})
}

func fetchLegacyMetadata(ctx context.Context, metadataURL string, jwtAud string) (*cfgcommonpb.ServiceDynamicMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request due to %w", err)
	}
	var authOpts []auth.RPCOption
	if jwtAud != "" {
		authOpts = append(authOpts, auth.WithIDTokenAudience(jwtAud))
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, authOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport %w", err)
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to %s due to %w", metadataURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch body, err := io.ReadAll(resp.Body); {
	case err != nil:
		return nil, fmt.Errorf("failed to read the response from %s: %w", metadataURL, err)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s returns %d. Body: %s", metadataURL, resp.StatusCode, body)
	default:
		ret := &cfgcommonpb.ServiceDynamicMetadata{}
		if err := protojson.Unmarshal(body, ret); err != nil {
			return nil, fmt.Errorf("failed to unmarshal ServiceDynamicMetadata: %w; Response body from %s: %s", err, metadataURL, body)
		}
		return ret, nil
	}
}

func skipUpdate(ctx context.Context, metadataEntity *model.ServiceMetadata, metadata *cfgcommonpb.ServiceMetadata, legacyMetadata *cfgcommonpb.ServiceDynamicMetadata) bool {
	switch {
	case metadataEntity == nil:
		panic(errors.New("nil metadataEntity"))
	case metadata != nil:
		return proto.Equal(metadataEntity.Metadata, metadata)
	case legacyMetadata != nil:
		return proto.Equal(metadataEntity.LegacyMetadata, legacyMetadata)
	default:
		logging.Warningf(ctx, "FIXME: both metadata and legacy metadata are nil")
		return false
	}
}
