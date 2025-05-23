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
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
)

// Update updates the `Service` entities for all registered services.
//
// Also deletes the entities for un-registered services.
func Update(ctx context.Context) error {
	servicesCfg := &cfgcommonpb.ServicesCfg{}
	if err := common.LoadSelfConfig(ctx, common.ServiceRegistryFilePath, servicesCfg); err != nil {
		return fmt.Errorf("failed to load %s. Reason: %w", common.ServiceRegistryFilePath, err)
	}

	toDelete, err := computeServicesToDelete(ctx, servicesCfg)
	if err != nil {
		return err
	}

	var errs []error
	var errorsMu sync.Mutex
	perr := parallel.WorkPool(8, func(workC chan<- func() error) {
		for _, srv := range servicesCfg.GetServices() {
			workC <- func() error {
				ctx := logging.SetField(ctx, "service", srv.GetId())
				if err := updateService(ctx, srv); err != nil {
					errorsMu.Lock()
					errs = append(errs, err)
					errorsMu.Unlock()
					logging.Errorf(ctx, "failed to update service. Reason: %s", err)
				}
				return nil
			}
		}

		if len(toDelete) > 0 {
			workC <- func() error {
				services := make([]string, len(toDelete))
				for i, key := range toDelete {
					services[i] = key.StringID()
				}
				if err := datastore.Delete(ctx, toDelete); err != nil {
					errs = append(errs, fmt.Errorf("failed to delete service(s) [%s]: %w", strings.Join(services, ", "), err))
					return nil
				}
				logging.Infof(ctx, "successfully deleted service(s): [%s]", strings.Join(services, ", "))
				return nil
			}
		}
	})

	if perr != nil {
		panic(fmt.Errorf("impossible pool error %w", perr))
	}
	return errors.Join(errs...)
}

func computeServicesToDelete(ctx context.Context, servicesCfg *cfgcommonpb.ServicesCfg) ([]*datastore.Key, error) {
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ServiceKind).KeysOnly(true), &keys); err != nil {
		return nil, fmt.Errorf("failed to query all service keys: %w", err)
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

func updateService(ctx context.Context, srvInfo *cfgcommonpb.Service) error {
	eg, ectx := errgroup.WithContext(ctx)
	updated := &model.Service{
		Name: srvInfo.GetId(),
		Info: srvInfo,
	}
	switch {
	case srvInfo.GetId() == info.AppID(ctx):
		eg.Go(func() error {
			metadata, err := getSelfMetadata(ctx)
			if err != nil {
				return err
			}
			if err := validateMetadata(metadata); err != nil {
				return fmt.Errorf("invalid metadata for self service: %w", err)
			}
			updated.Metadata = metadata
			return nil
		})
	case srvInfo.GetHostname() != "":
		eg.Go(func() error {
			metadata, err := fetchMetadata(ectx, srvInfo.GetHostname())
			if err != nil {
				return err
			}
			if err := validateMetadata(metadata); err != nil {
				return fmt.Errorf("invalid metadata for service %s: %w", srvInfo.GetId(), err)
			}
			updated.Metadata = metadata
			return nil
		})
	case srvInfo.GetMetadataUrl() != "":
		eg.Go(func() error {
			legacyMetadata, err := fetchLegacyMetadata(ectx, srvInfo.GetMetadataUrl(), srvInfo.GetJwtAuth().GetAudience())
			if err != nil {
				return err
			}
			if err := validateLegacyMetadata(legacyMetadata); err != nil {
				return fmt.Errorf("invalid legacy metadata for service %s: %w", srvInfo.GetId(), err)
			}
			updated.LegacyMetadata = legacyMetadata
			return nil
		})
	}

	var existing *model.Service
	eg.Go(func() error {
		service := &model.Service{
			Name: srvInfo.GetId(),
		}
		switch err := datastore.Get(ectx, service); err {
		case datastore.ErrNoSuchEntity:
			// Expect entity missing for the first time updating service.
			logging.Warningf(ectx, "missing Service datastore entity for %q. This is common for first time updating service", srvInfo.GetId())
		case nil:
			existing = service
		default:
			return err
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	if skipUpdate(existing, updated) {
		logging.Infof(ctx, "skip updating service as LUCI Config already has the latest")
		return nil
	}

	updated.UpdateTime = clock.Now(ctx).UTC()
	if err := datastore.Put(ctx, updated); err != nil {
		return err
	}
	logging.Infof(ctx, "successfully updated service")
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

func getSelfMetadata(ctx context.Context) (*cfgcommonpb.ServiceMetadata, error) {
	patterns, err := validation.Rules.ConfigPatterns(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect config patterns from self rules %w", err)
	}
	ret := &cfgcommonpb.ServiceMetadata{
		ConfigPatterns: make([]*cfgcommonpb.ConfigPattern, len(patterns)),
	}
	for i, p := range patterns {
		ret.ConfigPatterns[i] = &cfgcommonpb.ConfigPattern{
			ConfigSet: p.ConfigSet.String(),
			Path:      p.Path.String(),
		}
	}
	return ret, nil
}

func fetchLegacyMetadata(ctx context.Context, metadataURL string, jwtAud string) (*cfgcommonpb.ServiceDynamicMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request due to %w", err)
	}
	client := &http.Client{}
	if jwtAud != "" {
		if client.Transport, err = common.GetSelfSignedJWTTransport(ctx, jwtAud); err != nil {
			return nil, err
		}
	} else {
		if client.Transport, err = auth.GetRPCTransport(ctx, auth.AsSelf); err != nil {
			return nil, fmt.Errorf("failed to create transport %w", err)
		}
	}
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

func skipUpdate(existing, updated *model.Service) bool {
	return existing != nil &&
		existing.Name == updated.Name &&
		proto.Equal(existing.Info, updated.Info) &&
		proto.Equal(existing.Metadata, updated.Metadata) &&
		proto.Equal(existing.LegacyMetadata, updated.LegacyMetadata)
}
