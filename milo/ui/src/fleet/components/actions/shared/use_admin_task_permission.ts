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

import { useEffect, useState } from 'react';

import { logging } from '@/common/tools/logging';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

export function useAdminTaskPermission() {
  const fleetConsoleClient = useFleetConsoleClient();
  const [hasPermission, setHasPermission] = useState<boolean | null>(null);

  useEffect(() => {
    let isMounted = true;
    const checkPermission = async () => {
      try {
        const resp = await fleetConsoleClient.CheckAdminTaskPermission({});
        if (isMounted) {
          setHasPermission(resp.hasPermission);
        }
      } catch (e) {
        logging.error('Failed to check admin task permission:', e);
        if (isMounted) {
          setHasPermission(false);
        }
      }
    };
    void checkPermission();
    return () => {
      isMounted = false;
    };
  }, [fleetConsoleClient]);

  return hasPermission;
}
