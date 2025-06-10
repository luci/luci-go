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

import { Button } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { ListMachinesRequest } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

import { RecoverableLoggerErrorBoundary } from '../components/error_handling';
import { useUfsClient } from '../hooks/prpc_clients';
import { FleetHelmet } from '../layouts/fleet_helmet';
import { requestSurvey } from '../utils/survey';

export const SandboxPage = () => {
  const authState = useAuthState();

  // See go/luci-ui-rpc-tutorial for more info on how to make pRPC requests.
  const ufsClient = useUfsClient();

  const machines = useQuery({
    ...ufsClient.ListMachines.query(
      ListMachinesRequest.fromPartial({
        pageSize: 10,
      }),
    ),
    refetchInterval: 60000,
  });

  return (
    <>
      Welcome. This is a sandbox page with experiments and tools for developers
      of the Fleet Console to use for testing the functionality of the Fleet
      Console UI.
      <h2>Test Survey</h2>
      <Button
        onClick={() => requestSurvey(SETTINGS.fleetConsole.hats, authState)}
      >
        Test Survey
      </Button>
      <h2>Sample: UFS ListMachinesRequest</h2>
      <pre style={{ maxHeight: '300px', maxWidth: '100%', overflow: 'scroll' }}>
        {JSON.stringify(machines, null, 2)}
      </pre>
    </>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-sandbox">
      <FleetHelmet pageTitle="Sandbox" />
      <RecoverableLoggerErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-sandbox-page"
      >
        <SandboxPage />
      </RecoverableLoggerErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
