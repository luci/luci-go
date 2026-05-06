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
  BrowserReinstallConfig,
} from './request_repair_browser_config';
import { ChromeOSRepairConfig } from './request_repair_os_config';

/**
 * Configuration interface for repair/reinstall actions.
 * @template T The type of device items.
 */
export interface RepairConfig<T> {
  componentId: string | ((items: T[]) => string);
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
/**
 * Component to render repair and reinstall action buttons.
 * For Browser devices, it renders both "Request Repair" and "Request Reinstall".
 * For ChromeOS, it renders a single "Request Repair" button.
 */
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

  const fileRepairRequest = <T,>(config: RepairConfig<T>, items: T[]) => {
    let title = '';
    let description = '';
    let templateId = '';
    let hotlistIds = '';
    let componentId = '';

    try {
      componentId =
        typeof config.componentId === 'function'
          ? config.componentId(items)
          : config.componentId;
      title = config.generateTitle(items);
      description = config.generateDescription(items);
      templateId = config.getTemplateId(items);
      hotlistIds =
        typeof config.hotlistIds === 'function'
          ? config.hotlistIds(items)
          : config.hotlistIds || '';
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error((e as Error).message);
      return;
    }

    trackEvent('request_repair', {
      componentName: 'request_repair_button',
      dutCount: items.length,
      platform: platformToJSON(platform).toLowerCase(),
    });

    const params = new URLSearchParams({
      markdown: 'true',
      component: componentId,
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

  if (platform === Platform.CHROMEOS) {
    return (
      <Button
        data-testid="file-repair-bug-button"
        onClick={() =>
          fileRepairRequest(
            ChromeOSRepairConfig,
            selectedItems as DutToRepair[],
          )
        }
        color="primary"
        size="small"
        startIcon={<FeedbackOutlined />}
      >
        Request Repair
      </Button>
    );
  }

  const items = selectedItems as BrowserDeviceToRepair[];
  const actions = [
    {
      config: BrowserRepairConfig,
      label: 'Request Repair',
      testId: 'file-repair-bug-button',
    },
    {
      config: BrowserReinstallConfig,
      label: 'Request Reinstall',
      testId: 'file-reinstall-bug-button',
    },
  ];

  return (
    <>
      {actions.map((action, index) => (
        <Button
          key={action.label}
          data-testid={action.testId}
          onClick={() => fileRepairRequest(action.config, items)}
          color="primary"
          size="small"
          startIcon={<FeedbackOutlined />}
          sx={{ marginRight: index < actions.length - 1 ? 1 : 0 }}
        >
          {action.label}
        </Button>
      ))}
    </>
  );
}
