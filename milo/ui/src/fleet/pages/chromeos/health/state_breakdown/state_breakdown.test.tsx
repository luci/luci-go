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
  act,
  fireEvent,
  render,
  screen,
  waitForElementToBeRemoved,
} from '@testing-library/react';

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
import { HealthStatePopover } from './health_state_popover';
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

  it('opens Popover tooltip on hover', async () => {
    render(
      <SegmentedHealthBar
        title="brya"
        breakdown={mockBreakdown}
        testId="test-bar"
      />,
    );

    const bar = screen.getByTestId('test-bar');
    fireEvent.mouseEnter(bar);

    expect(await screen.findByText('1,000 total enrolled')).toBeInTheDocument();
    expect(screen.getByText('Availability')).toBeInTheDocument();
    expect(screen.getByText('Health')).toBeInTheDocument();
  });

  it('gracefully handles undefined or empty breakdown without crashing', () => {
    render(<SegmentedHealthBar title="empty-slice" testId="empty-bar" />);

    const bar = screen.getByTestId('empty-bar');
    expect(bar).toBeInTheDocument();
    expect(bar).toHaveAttribute('role', 'progressbar');
    expect(bar).toHaveAttribute(
      'aria-label',
      'empty-slice device health state distribution: No devices',
    );

    // Hover should not open popover when total is 0
    fireEvent.mouseEnter(bar);
    expect(screen.queryByText('total enrolled')).not.toBeInTheDocument();
  });

  it('renders breakdown segments when availability is 0% but devices exist in non-ready states', () => {
    const zeroAvailabilityBreakdown: DeviceStateBreakdown = {
      total: 15,
      ready: 0,
      needsRepair: 10,
      repairFailed: 5,
      needsManualRepair: 0,
      needsReplacement: 0,
      reserved: 0,
      needsDeploy: 0,
    };

    render(
      <SegmentedHealthBar
        title="zero-ready-model"
        breakdown={zeroAvailabilityBreakdown}
        availabilityPct={0}
        testId="zero-ready-bar"
      />,
    );

    const bar = screen.getByTestId('zero-ready-bar');
    expect(bar).toBeInTheDocument();

    // Breakdown segments should be rendered according to their real states
    expect(screen.getByTestId('bar-segment-needs_repair')).toBeInTheDocument();
    expect(screen.getByTestId('bar-segment-repair_failed')).toBeInTheDocument();

    // Accessible description should preserve state distribution
    expect(bar).toHaveAttribute(
      'aria-label',
      expect.stringContaining(
        'Needs repair: 10 (66.7%), Repair failed: 5 (33.3%)',
      ),
    );
  });
});

describe('HealthStatePopover', () => {
  it('wraps arbitrary child and opens state popover on hover', async () => {
    render(
      <HealthStatePopover
        title="brya"
        breakdown={mockBreakdown}
        availabilityPct={95.4}
      >
        <span data-testid="test-label">Healthy</span>
      </HealthStatePopover>,
    );

    const label = screen.getByTestId('test-label');
    expect(label).toHaveTextContent('Healthy');

    fireEvent.mouseEnter(label);
    expect(await screen.findByText('brya')).toBeInTheDocument();
    expect(screen.getByTestId('popover-thresholds-strip')).toBeInTheDocument();
    expect(screen.getByTestId('popover-threshold-healthy')).toBeInTheDocument();
    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();
  });

  it('toggles popover open and closed on click', async () => {
    render(
      <HealthStatePopover
        title="brya"
        breakdown={mockBreakdown}
        transitionDuration={0}
      >
        <button type="button" data-testid="test-btn">
          Click me
        </button>
      </HealthStatePopover>,
    );

    const btn = screen.getByTestId('test-btn');
    fireEvent.click(btn);
    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();

    fireEvent.click(btn);
    await waitForElementToBeRemoved(() =>
      screen.queryByText('1,000 total enrolled'),
    );
  });

  it('does not open when disabled is true', () => {
    render(
      <HealthStatePopover title="brya" breakdown={mockBreakdown} disabled>
        <span data-testid="disabled-label">Disabled</span>
      </HealthStatePopover>,
    );

    const label = screen.getByTestId('disabled-label');
    fireEvent.mouseEnter(label);
    expect(screen.queryByText('1,000 total enrolled')).not.toBeInTheDocument();
  });

  it('closes popover immediately when a wheel or scroll event occurs', async () => {
    render(
      <HealthStatePopover
        title="brya"
        breakdown={mockBreakdown}
        transitionDuration={0}
      >
        <button type="button" data-testid="scroll-btn">
          Click me
        </button>
      </HealthStatePopover>,
    );

    const btn = screen.getByTestId('scroll-btn');
    fireEvent.click(btn);
    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();

    // Trigger wheel on window
    fireEvent.wheel(window);

    await waitForElementToBeRemoved(() =>
      screen.queryByText('1,000 total enrolled'),
    );
  });

  it('respects enterDelay and cancels opening when cursor leaves before delay', () => {
    jest.useFakeTimers();
    try {
      render(
        <HealthStatePopover
          title="brya"
          breakdown={mockBreakdown}
          enterDelay={150}
        >
          <span data-testid="delay-target">Hover target</span>
        </HealthStatePopover>,
      );

      const target = screen.getByTestId('delay-target');
      fireEvent.mouseEnter(target);

      // Immediately, popover is not in document
      expect(
        screen.queryByText('1,000 total enrolled'),
      ).not.toBeInTheDocument();

      // Mouse leaves after 50ms (before 150ms delay)
      act(() => {
        jest.advanceTimersByTime(50);
      });
      fireEvent.mouseLeave(target);

      // Advance time past the 150ms mark
      act(() => {
        jest.advanceTimersByTime(200);
      });
      expect(
        screen.queryByText('1,000 total enrolled'),
      ).not.toBeInTheDocument();

      // Now enter again and stay past 150ms
      fireEvent.mouseEnter(target);
      act(() => {
        jest.advanceTimersByTime(160);
      });
      expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();
    } finally {
      jest.useRealTimers();
    }
  });
});

describe('HealthStatePopoverContent', () => {
  it('renders header metrics, categorized sections, and 1-click links', () => {
    render(
      <HealthStatePopoverContent
        title="volteer"
        breakdown={mockBreakdown}
        baseFilter='model = "volteer"'
      />,
    );

    expect(screen.getByText('volteer')).toBeInTheDocument();
    expect(screen.getByText('1,000 total enrolled')).toBeInTheDocument();
    expect(screen.getAllByText('94.5%').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Availability')).toBeInTheDocument();
    expect(screen.getByText('Health')).toBeInTheDocument();

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

  it('renders explicit availabilityPct formatted to 1 decimal place alongside healthPct', () => {
    // Availability from SLA/Quota trendline point (e.g. 98.52345% -> formatted to 98.5%)
    // Health computed from breakdown (945 / 1000 = 94.5%)
    render(
      <HealthStatePopoverContent
        title="brya"
        breakdown={mockBreakdown}
        availabilityPct={98.52345}
      />,
    );

    expect(screen.getByText('98.5%')).toBeInTheDocument();
    expect(screen.getByText('94.5%')).toBeInTheDocument();
    expect(screen.queryByText(/target/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/fleet/i)).not.toBeInTheDocument();
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
    expect(
      screen.queryByTestId('state-row-registered'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('state-row-unknown')).not.toBeInTheDocument();
  });

  it('renders SLA thresholds strip with healthy, warning, and critical tiers', () => {
    // mockBreakdown has 945 healthy out of 1000 = 94.5% (Warning tier: 80%–94%)
    render(
      <HealthStatePopoverContent title="brya" breakdown={mockBreakdown} />,
    );

    const strip = screen.getByTestId('popover-thresholds-strip');
    expect(strip).toBeInTheDocument();
    expect(strip).toHaveTextContent('SLA');

    const healthyTier = screen.getByTestId('popover-threshold-healthy');
    expect(healthyTier).toBeInTheDocument();
    expect(healthyTier).toHaveTextContent('Healthy ≥ 95%');

    const warningTier = screen.getByTestId('popover-threshold-warning');
    expect(warningTier).toBeInTheDocument();
    expect(warningTier).toHaveTextContent('Warning 80%–94%');

    const criticalTier = screen.getByTestId('popover-threshold-critical');
    expect(criticalTier).toBeInTheDocument();
    expect(criticalTier).toHaveTextContent('Critical < 80%');
  });

  it('renders empty state when total enrolled devices is 0', () => {
    render(
      <HealthStatePopoverContent
        title="empty-model"
        breakdown={{
          total: 0,
          ready: 0,
          needsRepair: 0,
          repairFailed: 0,
          needsManualRepair: 0,
          needsReplacement: 0,
          reserved: 0,
          needsDeploy: 0,
        }}
      />,
    );

    const emptyBox = screen.getByTestId('popover-empty-state');
    expect(emptyBox).toBeInTheDocument();
    expect(screen.getByText('No devices enrolled')).toBeInTheDocument();
    expect(
      screen.queryByText('HEALTHY (OPERABLE CAPACITY)'),
    ).not.toBeInTheDocument();
  });
});
