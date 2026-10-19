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

import { fireEvent, render, screen } from '@testing-library/react';

import { DeviceStateCounts } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  BreakdownStats,
  computeBreakdownStats,
  convertProtoCountsToBreakdown,
  DeviceStateBreakdown,
} from './compute_breakdown_stats';
import {
  HEALTH_CATEGORIES,
  STATE_VISUAL_CONFIGS,
} from './health_state_definitions';
import { HealthStatePopoverContent } from './health_state_popover_content';
import { SegmentedHealthBar } from './segmented_health_bar';
import {
  adaptHealthFilterToDeviceList,
  getDeviceListStateUrl,
} from './state_navigation_utils';

const mockBreakdown: DeviceStateBreakdown = {
  total: 1000,
  ready: 850,
  needsRepair: 60,
  repairFailed: 20,
  needsManualRepair: 40,
  needsReplacement: 10,
  reserved: 15,
  needsDeploy: 5,
};

describe('health_state_definitions', () => {
  it('defines valid color tokens and categories for all DUT states', () => {
    expect(STATE_VISUAL_CONFIGS.reserved.color).toBe('#1b5e20');
    expect(HEALTH_CATEGORIES.healthy.states).toContain('reserved');

    expect(STATE_VISUAL_CONFIGS.ready.color).toBe('#43a047');
    expect(HEALTH_CATEGORIES.healthy.states).toContain('ready');

    expect(STATE_VISUAL_CONFIGS.needs_repair.color).toBe('#81c784');
    expect(HEALTH_CATEGORIES.healthy.states).toContain('needs_repair');

    expect(STATE_VISUAL_CONFIGS.repair_failed.color).toBe('#81c784');
    expect(HEALTH_CATEGORIES.healthy.states).toContain('repair_failed');

    expect(STATE_VISUAL_CONFIGS.needs_deploy.color).toBe('#d32f2f');
    expect(HEALTH_CATEGORIES.action_required.states).toContain('needs_deploy');

    expect(STATE_VISUAL_CONFIGS.needs_manual_repair.color).toBe('#d32f2f');
    expect(HEALTH_CATEGORIES.action_required.states).toContain(
      'needs_manual_repair',
    );

    expect(STATE_VISUAL_CONFIGS.needs_replacement.color).toBe('#d32f2f');
    expect(HEALTH_CATEGORIES.action_required.states).toContain(
      'needs_replacement',
    );

    expect(STATE_VISUAL_CONFIGS.registered.color).toBe('#757575');
    expect(HEALTH_CATEGORIES.administrative.states).toContain('registered');

    expect(STATE_VISUAL_CONFIGS.unknown.color).toBe('#9e9e9e');
    expect(HEALTH_CATEGORIES.administrative.states).toContain('unknown');
  });
});

describe('compute_breakdown_stats', () => {
  it('correctly converts protobuf DeviceStateCounts to DeviceStateBreakdown', () => {
    const protoCounts: DeviceStateCounts = {
      ready: 500,
      needRepair: 30,
      repairFailed: 10,
      needManualRepair: 20,
      needsReplacement: 5,
      reserved: 12,
      needsDeploy: 8,
    };

    const breakdown = convertProtoCountsToBreakdown(protoCounts);
    expect(breakdown).toBeDefined();
    expect(breakdown?.total).toBe(585);
    expect(breakdown?.ready).toBe(500);
    expect(breakdown?.needsRepair).toBe(30);
    expect(breakdown?.repairFailed).toBe(10);
    expect(breakdown?.needsManualRepair).toBe(20);
    expect(breakdown?.needsReplacement).toBe(5);
    expect(breakdown?.reserved).toBe(12);
    expect(breakdown?.needsDeploy).toBe(8);
  });

  it('returns undefined if protobuf DeviceStateCounts is null or has zero total', () => {
    expect(convertProtoCountsToBreakdown(null)).toBeUndefined();
    expect(convertProtoCountsToBreakdown(undefined)).toBeUndefined();
    expect(
      convertProtoCountsToBreakdown({
        ready: 0,
        needRepair: 0,
        repairFailed: 0,
        needManualRepair: 0,
        needsReplacement: 0,
        reserved: 0,
        needsDeploy: 0,
      }),
    ).toBeUndefined();
  });

  it('correctly handles fractional state distribution (1-device pool scenario)', () => {
    const fractionalCounts = {
      ready: 0.7,
      needRepair: 0.3,
      repairFailed: 0,
      needManualRepair: 0,
      needsReplacement: 0,
      reserved: 0,
      needsDeploy: 0,
    };

    const breakdown = convertProtoCountsToBreakdown(fractionalCounts);
    expect(breakdown).toBeDefined();
    expect(breakdown?.total).toBe(1);
    expect(breakdown?.ready).toBe(0.7);
    expect(breakdown?.needsRepair).toBe(0.3);

    const stats: BreakdownStats = computeBreakdownStats(breakdown!);
    expect(stats.healthyPct).toBe(70);
    expect(stats.recoveringPct).toBe(30);
    expect(stats.healthyTotalPct).toBe(100);
  });

  it('computes breakdown stats and combines auto-healing and reserved into healthy total', () => {
    const stats: BreakdownStats = computeBreakdownStats(mockBreakdown);

    expect(stats.total).toBe(1000);
    expect(stats.healthyPct).toBe(85); // 850 ready
    expect(stats.recoveringPct).toBe(8); // 80 in auto recovery (60 + 20)
    expect(stats.brokenPct).toBe(5.5); // 55 action required (5 deploy + 40 manual repair + 10 replacement)

    // Domain truth: healthy capacity includes ready + recovering (self-healing) + reserved
    expect(stats.healthyTotalCount).toBe(945); // 850 + 60 + 20 + 15
    expect(stats.healthyTotalPct).toBe(94.5);
    expect(stats.unhealthyTotalCount).toBe(55); // 5 + 40 + 10
    expect(stats.unhealthyTotalPct).toBe(5.5);
  });
});

describe('state_navigation_utils', () => {
  it('adapts health filters (model, pool, device_id) to device list syntax', () => {
    expect(adaptHealthFilterToDeviceList('model = "brya"')).toBe(
      'labels.model = "brya"',
    );
    expect(adaptHealthFilterToDeviceList('pool = "DUT_POOL_QUOTA"')).toBe(
      'labels.pool = "DUT_POOL_QUOTA"',
    );
    expect(adaptHealthFilterToDeviceList('device_id = "cros-1"')).toBe(
      'id = "cros-1"',
    );
    // Already adapted labels remain untouched
    expect(adaptHealthFilterToDeviceList('labels.model = "brya"')).toBe(
      'labels.model = "brya"',
    );
    expect(adaptHealthFilterToDeviceList('')).toBe('');
    expect(adaptHealthFilterToDeviceList(undefined)).toBe('');
  });

  it('generates correct AIP-160 device list URL with and without baseFilter', () => {
    const urlWithoutBase = getDeviceListStateUrl('needs_repair');
    expect(urlWithoutBase).toBe(
      '/ui/fleet/p/chromeos/devices?filters=labels.%22dut_state%22%20%3D%20%22needs_repair%22',
    );

    const urlWithBase = getDeviceListStateUrl(
      'needs_manual_repair',
      'model = "brya"',
    );
    const expectedParam =
      '(labels.model%20%3D%20%22brya%22)%20AND%20' +
      '(labels.%22dut_state%22%20%3D%20%22needs_manual_repair%22)';
    expect(urlWithBase).toBe(
      `/ui/fleet/p/chromeos/devices?filters=${expectedParam}`,
    );
  });
});

describe('SegmentedHealthBar', () => {
  it('renders progress bar segments with tonal colors', () => {
    render(
      <SegmentedHealthBar
        title="brya"
        breakdown={mockBreakdown}
        testId="test-bar"
      />,
    );

    const bar = screen.getByTestId('test-bar');
    expect(bar).toBeInTheDocument();
    expect(bar).toHaveAttribute('role', 'progressbar');

    // Segments should be present for positive counts
    expect(screen.getByTestId('bar-segment-ready')).toBeInTheDocument();
    expect(screen.getByTestId('bar-segment-needs_repair')).toBeInTheDocument();
    expect(screen.getByTestId('bar-segment-repair_failed')).toBeInTheDocument();
    expect(
      screen.getByTestId('bar-segment-needs_manual_repair'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('bar-segment-needs_replacement'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('bar-segment-reserved')).toBeInTheDocument();
    expect(screen.getByTestId('bar-segment-needs_deploy')).toBeInTheDocument();
  });

  it('opens Popover tooltip on hover', () => {
    render(
      <SegmentedHealthBar
        title="brya"
        breakdown={mockBreakdown}
        testId="test-bar"
      />,
    );

    const bar = screen.getByTestId('test-bar');
    fireEvent.mouseEnter(bar);

    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();
    expect(screen.getByText('Availability')).toBeInTheDocument();
    expect(screen.getByText('Health')).toBeInTheDocument();
  });
});

describe('HealthStatePopoverContent', () => {
  it('renders header metrics, categorized sections, and 1-click links', () => {
    render(
      <HealthStatePopoverContent
        title="volteer"
        breakdown={mockBreakdown}
        baseFilter='model = "volteer"'
        targetAvailabilityPct={90}
      />,
    );

    expect(screen.getByText('volteer')).toBeInTheDocument();
    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();
    expect(screen.getAllByText('94.5%').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Availability')).toBeInTheDocument();
    expect(screen.getByText('Health')).toBeInTheDocument();

    // Target quota
    expect(screen.getByText('Expected Quota Target')).toBeInTheDocument();

    // Macro category headers
    expect(screen.getByText('Healthy (Operable Capacity)')).toBeInTheDocument();
    expect(screen.getByText('Unhealthy (Action Required)')).toBeInTheDocument();

    // Exact state rows
    expect(screen.getByTestId('state-row-ready')).toHaveAttribute(
      'href',
      expect.stringContaining('dut_state%22%20%3D%20%22ready%22'),
    );
    expect(screen.getByTestId('state-row-needs_repair')).toHaveAttribute(
      'href',
      expect.stringContaining('dut_state%22%20%3D%20%22needs_repair%22'),
    );
    expect(screen.getByTestId('state-row-needs_manual_repair')).toHaveAttribute(
      'href',
      expect.stringContaining('dut_state%22%20%3D%20%22needs_manual_repair%22'),
    );
    expect(screen.getByTestId('state-row-reserved')).toHaveAttribute(
      'href',
      expect.stringContaining('dut_state%22%20%3D%20%22reserved%22'),
    );
    expect(screen.getByTestId('state-row-needs_deploy')).toHaveAttribute(
      'href',
      expect.stringContaining('dut_state%22%20%3D%20%22needs_deploy%22'),
    );

    // "View all devices" footer button
    const viewAllBtn = screen.getByRole('link', { name: /view all devices/i });
    expect(viewAllBtn).toHaveAttribute(
      'href',
      '/ui/fleet/p/chromeos/devices?filters=labels.model%20%3D%20%22volteer%22',
    );
  });

  it('calculates availability using expectedCount as denominator from the exact same healthy numerator', () => {
    // 945 healthy devices out of 1000 total devices
    // Quota expectedCount = 950
    // Availability = 945 / 950 * 100 = 99.5%
    // Health = 945 / 1000 * 100 = 94.5%
    render(
      <HealthStatePopoverContent
        title="brya"
        breakdown={mockBreakdown}
        expectedCount={950}
      />,
    );

    expect(screen.getByText('99.5%')).toBeInTheDocument();
    expect(screen.getByText('94.5%')).toBeInTheDocument();
    expect(screen.getByText('945 / 950 target')).toBeInTheDocument();
    expect(screen.getByText('945 / 1,000 fleet')).toBeInTheDocument();
  });

  it('renders "Other states" section when registered or unknown devices are present', () => {
    const breakdownWithOther: DeviceStateBreakdown = {
      ...mockBreakdown,
      registered: 12,
      unknown: 8,
    };

    render(
      <HealthStatePopoverContent title="brya" breakdown={breakdownWithOther} />,
    );

    expect(screen.getByText('Other states')).toBeInTheDocument();
    expect(screen.getByTestId('state-row-registered')).toBeInTheDocument();
    expect(screen.getByTestId('state-row-unknown')).toBeInTheDocument();
  });

  it('renders "Other states" section with 0 count when registered and unknown devices are not present', () => {
    render(
      <HealthStatePopoverContent title="brya" breakdown={mockBreakdown} />,
    );

    expect(screen.getByText('Other states')).toBeInTheDocument();
    const registeredRow = screen.getByTestId('state-row-registered');
    expect(registeredRow).toBeInTheDocument();
    expect(registeredRow).toHaveTextContent('0');

    const unknownRow = screen.getByTestId('state-row-unknown');
    expect(unknownRow).toBeInTheDocument();
    expect(unknownRow).toHaveTextContent('0');
  });
});
