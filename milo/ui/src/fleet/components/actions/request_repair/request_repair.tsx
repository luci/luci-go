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
import { FeedbackOutlined } from '@mui/icons-material';
import { Button } from '@mui/material';
import type { ReactElement } from 'react';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import {
  Platform,
  platformToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { DutToRepair } from '../shared/types';

import {
  BrowserDeviceToRepair,
  BrowserRepairConfig,
} from './request_repair_browser_config';
import { ChromeOSRepairConfig } from './request_repair_os_config';

export interface RepairConfig<T> {
  componentId: string;
  getTemplateId: (items: T[]) => string;
  generateTitle: (items: T[]) => string;
  generateDescription: (items: T[]) => string;
  hotlistIds?: string | ((items: T[]) => string);
}

// Function overloads to provide type safety for callers based on the platform.
export function RequestRepair(props: {
  selectedItems: DutToRepair[];
  platform: Platform.CHROMEOS;
}): ReactElement;
export function RequestRepair(props: {
  selectedItems: BrowserDeviceToRepair[];
  platform: Platform.CHROMIUM;
}): ReactElement;
export function RequestRepair({
  selectedItems,
  platform,
}: {
  selectedItems: DutToRepair[] | BrowserDeviceToRepair[];
  platform: Platform.CHROMEOS | Platform.CHROMIUM;
}) {
  const { trackEvent } = useGoogleAnalytics();
  const showButton = selectedItems?.length > 0;

  if (!showButton) {
    return <></>;
  }

  const fileRepairRequest = () => {
    let title = '';
    let description = '';
    let templateId = '';
    let config;

    let hotlistIds = '';

    if (platform === Platform.CHROMEOS) {
      const items = selectedItems as DutToRepair[];
      config = ChromeOSRepairConfig;
      title = config.generateTitle(items);
      description = config.generateDescription(items);
      templateId = config.getTemplateId(items);
      hotlistIds =
        typeof config.hotlistIds === 'function'
          ? config.hotlistIds(items)
          : config.hotlistIds || '';
    } else {
      const items = selectedItems as BrowserDeviceToRepair[];
      config = BrowserRepairConfig;
      title = config.generateTitle(items);
      description = config.generateDescription(items);
      templateId = config.getTemplateId(items);
      hotlistIds =
        typeof config.hotlistIds === 'function'
          ? config.hotlistIds(items)
          : config.hotlistIds || '';
    }

    trackEvent('request_repair', {
      componentName: 'request_repair_button',
      dutCount: selectedItems.length,
      platform: platformToJSON(platform).toLowerCase(),
    });

    const params = new URLSearchParams({
      markdown: 'true',
      component: config.componentId,
      template: templateId,
      title,
      description,
    });
    if (hotlistIds) {
      params.set('hotlistIds', hotlistIds);
    }
    const url = `http://b/issues/new?${params.toString()}`;
    window.open(url, '_blank');
  };

  return (
    <Button
      data-testid="file-repair-bug-button"
      onClick={fileRepairRequest}
      color="primary"
      size="small"
      startIcon={<FeedbackOutlined />}
    >
      Request Repair
    </Button>
  );
}
