// Copyright 2026 The LUCI Authors.
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

import {
  CreatePhysicalSiteRequest,
  CreatePhysicalSiteResponse,
  DeletePhysicalSiteRequest,
  DeletePhysicalSiteResponse,
  FleetConsole,
  ListPhysicalSitesRequest,
  ListPhysicalSitesResponse,
  PhysicalSite,
  UpdatePhysicalSiteRequest,
  UpdatePhysicalSiteResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { rpc, RpcJsonHandler } from './fake_priority_rules';

export function createPhysicalSitesFake() {
  const sites = new Map<string, PhysicalSite>();

  const sortSites = (items: Iterable<PhysicalSite>): PhysicalSite[] =>
    Array.from(items).sort(
      (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
    );

  const service: Pick<
    FleetConsole,
    | 'ListPhysicalSites'
    | 'CreatePhysicalSite'
    | 'UpdatePhysicalSite'
    | 'DeletePhysicalSite'
  > = {
    async ListPhysicalSites(_req: ListPhysicalSitesRequest) {
      return ListPhysicalSitesResponse.create({
        physicalSites: sortSites(sites.values()),
      });
    },

    async CreatePhysicalSite(req: CreatePhysicalSiteRequest) {
      const incoming = req.physicalSite;
      const id = incoming?.id?.trim() ?? '';
      const name = incoming?.name?.trim() ?? '';
      const created = PhysicalSite.create({ id, name });
      if (created.id) {
        sites.set(created.id, created);
      }
      return CreatePhysicalSiteResponse.create({ physicalSite: created });
    },

    async UpdatePhysicalSite(req: UpdatePhysicalSiteRequest) {
      const incoming = req.physicalSite;
      const id = incoming?.id?.trim() ?? '';
      const existing = sites.get(id);
      if (!existing) {
        return UpdatePhysicalSiteResponse.create({});
      }
      const updated = PhysicalSite.create({
        id,
        name: incoming?.name?.trim() || existing.name,
      });
      sites.set(updated.id, updated);
      return UpdatePhysicalSiteResponse.create({ physicalSite: updated });
    },

    async DeletePhysicalSite(req: DeletePhysicalSiteRequest) {
      sites.delete(req.id.trim());
      return DeletePhysicalSiteResponse.create({});
    },
  };

  return {
    ...service,
    handlers: {
      ListPhysicalSites: rpc(
        ListPhysicalSitesRequest,
        ListPhysicalSitesResponse,
        service.ListPhysicalSites,
      ),
      CreatePhysicalSite: rpc(
        CreatePhysicalSiteRequest,
        CreatePhysicalSiteResponse,
        service.CreatePhysicalSite,
      ),
      UpdatePhysicalSite: rpc(
        UpdatePhysicalSiteRequest,
        UpdatePhysicalSiteResponse,
        service.UpdatePhysicalSite,
      ),
      DeletePhysicalSite: rpc(
        DeletePhysicalSiteRequest,
        DeletePhysicalSiteResponse,
        service.DeletePhysicalSite,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    sites,
    seed(nextSites: readonly PhysicalSite[]) {
      sites.clear();
      for (const s of nextSites) {
        const normalized = PhysicalSite.create({
          id: s.id.trim(),
          name: s.name.trim(),
        });
        sites.set(normalized.id, normalized);
      }
    },
  };
}
