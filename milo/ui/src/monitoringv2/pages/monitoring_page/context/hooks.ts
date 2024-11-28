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

import { useContext } from 'react';

import { MonitoringCtx } from './context';

export function useTree() {
  const context = useContext(MonitoringCtx);

  if (!context) {
    throw new Error('useTree can only be used in a MonitoringProvider');
  }

  return context.tree;
}

export function useAlerts() {
  const context = useContext(MonitoringCtx);

  if (!context) {
    throw new Error('useAlerts can only be used in a MonitoringProvider');
  }

  return {
    // FIXME: remove alerts.
    alerts: context.alerts,
    builderAlerts: context.builderAlerts,
    stepAlerts: context.stepAlerts,
    testAlerts: context.testAlerts,
    alertsLoading: context.alertsLoading,
    alertsLoadingStatus: context.alertsLoadingStatus,
  };
}

export function useBugs() {
  const context = useContext(MonitoringCtx);

  if (!context) {
    throw new Error('useBugs can only be used in a MonitoringProvider');
  }

  return {
    bugs: context.bugs,
    bugsError: context.bugsError,
    bugsLoading: context.bugsLoading,
    isBugsError: context.isBugsError,
  };
}
