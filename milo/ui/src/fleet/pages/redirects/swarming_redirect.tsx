// Copyright 2025 The LUCI Authors.
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

import { Alert, AlertTitle, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Navigate, To, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { DecoratedClient } from '@/common/hooks/prpc_query';
import { genFeedbackUrl } from '@/common/tools/utils';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  BotRequest,
  BotsClientImpl,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

export function SwarmingRedirect() {
  const params = useParams();
  const [searchParams] = useSyncedSearchParams();

  const client = useBotsClient(DEVICE_TASKS_SWARMING_HOST);
  const q = useQuery({
    queryFn: () => getRedirectAddress(params['*'], searchParams, client),
  });

  if (q.isLoading) return 'Redirecting...';

  if (q.isError)
    return (
      <Alert severity="error">
        <AlertTitle>Error with redirection</AlertTitle>
        <p>{q.error instanceof Error ? q.error.message : 'Unknown error'}</p>
        <p>
          If you believe that this should have worked, let us know by submitting
          your{' '}
          <Link
            href={genFeedbackUrl({
              bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
              errMsg: `Error with redirection`,
            })}
            target="_blank"
          >
            feedback
          </Link>
          !
        </p>
      </Alert>
    );

  return <Navigate to={q.data} />;
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-swarming-redirect">
      <FleetHelmet pageTitle="Swarming redirect" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-swarming-redirect-page"
      >
        <SwarmingRedirect />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

const pathPrefix = '/ui/fleet/labs';
/* Maps swarming path to fleet console paths.
 * Supports:
 *    https://chromeos-swarming.appspot.com/botlist
 *    https://chromeos-swarming.appspot.com/bot?id={BOT_ID}
 * */
const getRedirectAddress = async (
  url: string | undefined,
  searchParams: URLSearchParams,
  swarmingClient: DecoratedClient<BotsClientImpl>,
): Promise<To> => {
  switch (url) {
    case 'botlist':
      return { pathname: `${pathPrefix}/devices` };
    case 'bot': {
      const bot_id = searchParams.get('id');
      if (!bot_id) throw Error(`Missing bot id`);

      const res = await swarmingClient.GetBot(
        BotRequest.fromPartial({ botId: bot_id }),
      );

      const dutName = res.dimensions.find(
        ({ key }) => key === 'dut_name',
      )?.value;
      if (!dutName) throw Error(`Cannot find dut_name of device ${bot_id}`);

      return { pathname: `${pathPrefix}/devices/${dutName}` };
    }
  }

  throw Error('No page mapping found');
};
