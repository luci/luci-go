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
  FleetConsole,
  IrmIncident,
  ListIrmIncidentsRequest,
  ListIrmIncidentsResponse,
  UpdateIrmIncidentRequest,
  UpdateIrmIncidentResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { rpc, RpcJsonHandler } from './fake_priority_rules';

export function createIrmFake() {
  const incidents = new Map<string, IrmIncident>();

  const service: Pick<FleetConsole, 'ListIrmIncidents' | 'UpdateIrmIncident'> =
    {
      async ListIrmIncidents(_req: ListIrmIncidentsRequest) {
        return ListIrmIncidentsResponse.create({
          irmIncidents: Array.from(incidents.values()),
        });
      },

      async UpdateIrmIncident(req: UpdateIrmIncidentRequest) {
        const existing = incidents.get(req.id);
        const updated = IrmIncident.create({
          id: req.id,
          masterBugId: existing?.masterBugId ?? '',
          title: existing?.title ?? '',
          expressionsAip160: [...(req.expressionsAip160 ?? [])],
          affectedDevicesCount: existing?.affectedDevicesCount ?? 0,
        });
        incidents.set(updated.id, updated);
        return UpdateIrmIncidentResponse.create({ irmIncident: updated });
      },
    };

  return {
    ...service,
    handlers: {
      ListIrmIncidents: rpc(
        ListIrmIncidentsRequest,
        ListIrmIncidentsResponse,
        service.ListIrmIncidents,
      ),
      UpdateIrmIncident: rpc(
        UpdateIrmIncidentRequest,
        UpdateIrmIncidentResponse,
        service.UpdateIrmIncident,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    incidents,
    seed(nextIncidents: readonly IrmIncident[]) {
      incidents.clear();
      for (const inc of nextIncidents) {
        const normalized = IrmIncident.create(inc);
        incidents.set(normalized.id, normalized);
      }
    },
  };
}
