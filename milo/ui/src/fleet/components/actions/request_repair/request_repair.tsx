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
import { Button, Tooltip } from '@mui/material';
import type { ReactElement } from 'react';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import {
  AndroidDevice,
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
import {
  areDevicesFromSingleLab,
  extractPixelLabName,
  PixelRestorationConfig,
} from './request_repair_pixel_config';

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
  baseUrl?: string;
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
export function RequestRepair(props: {
  selectedItems: AndroidDevice[];
  platform: Platform.PIXEL;
}): ReactElement;
/**
 * Component to render repair, reinstall, and restoration action buttons.
 * For Browser devices, it renders both "Request Repair" and "Request Reinstall".
 * For ChromeOS, it renders a single "Request Repair" button.
 * For Pixel, it renders a "Request Restoration" button restricted to a single lab.
 */
export function RequestRepair({
  selectedItems,
  platform,
}: {
  selectedItems: DutToRepair[] | BrowserDeviceToRepair[] | AndroidDevice[];
  platform: Platform.CHROMEOS | Platform.CHROMIUM | Platform.PIXEL;
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
      componentName:
        platform === Platform.PIXEL
          ? 'request_restoration_button'
          : 'request_repair_button',
      dutCount: items.length,
      platform: platformToJSON(platform).toLowerCase(),
    });

    const params = new URLSearchParams({
      format: 'MARKDOWN',
      component: componentId,
      template: templateId,
      title,
      description,
    });
    if (hotlistIds) {
      params.set('hotlistIds', hotlistIds);
    }
    const baseUrl = config.baseUrl || 'http://b';
    const url = `${baseUrl}/issues/new?${params.toString()}`;
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

  if (platform === Platform.PIXEL) {
    const devices = selectedItems as AndroidDevice[];
    const isSingleLab = areDevicesFromSingleLab(devices);
    const hasMissingLab = devices.some((d) => !extractPixelLabName(d));
    const labName =
      isSingleLab && devices.length > 0 ? extractPixelLabName(devices[0]) : '';

    let tooltipTitle: string;
    if (hasMissingLab) {
      tooltipTitle = 'All selected devices must belong to a known lab';
    } else if (!isSingleLab) {
      tooltipTitle = 'Selected devices must belong to the same lab';
    } else {
      tooltipTitle = `Create restoration bug for ${devices.length} ${devices.length === 1 ? 'device' : 'devices'} in ${labName}`;
    }

    return (
      <Tooltip title={tooltipTitle}>
        <span data-testid="request-restoration-button">
          <Button
            data-testid="file-restoration-bug-button"
            disabled={!isSingleLab}
            onClick={() => {
              if (!isSingleLab) return;
              fileRepairRequest(PixelRestorationConfig, devices);
            }}
            color="primary"
            size="small"
            startIcon={<FeedbackOutlined />}
          >
            Request Restoration
          </Button>
        </span>
      </Tooltip>
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
