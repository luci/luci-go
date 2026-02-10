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

import { useQuery } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ResourceRequestTable } from './resource_requests_table';

// Mock dependencies
jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(() => ({
    ListResourceRequests: {
      query: jest.fn(() => ({ queryKey: ['list'], queryFn: jest.fn() })),
    },
    GetResourceRequestsMultiselectFilterValues: {
      query: jest.fn(() => ({ queryKey: ['filters'], queryFn: jest.fn() })),
    },
  })),
}));

jest.mock('@tanstack/react-query', () => {
  const original = jest.requireActual('@tanstack/react-query');
  return {
    ...original,
    useQuery: jest.fn(),
  };
});

describe('<ResourceRequestTable />', () => {
  const mockResourceRequests = [
    {
      resourceDetails: 'Laptop 1',
      rrId: 'rr-123',
      resource_request_bug_id: '12345',
      resourceGroups: ['group1'],
      resourceRequestTargetDeliveryDate: { year: 2023, month: 1, day: 1 },
      resourceRequestActualDeliveryDate: { year: 2023, month: 1, day: 5 },
      fulfillmentStatus: 1, // FULFILLED or similar
      resourceRequestStatus: 1,
      resourceRequestBugStatus: 'Fixed',
      procurementActualDeliveryDate: undefined,
      procurementTargetDeliveryDate: undefined,
      buildActualDeliveryDate: undefined,
      buildTargetDeliveryDate: undefined,
      qaActualDeliveryDate: undefined,
      qaTargetDeliveryDate: undefined,
      configActualDeliveryDate: undefined,
      configTargetDeliveryDate: undefined,
      customer: 'Customer A',
      resourceName: 'Pixel 7',
      acceptedQuantity: 10,
      criticality: 'High',
      requestApproval: 'Approved',
      resourcePm: 'PM A',
      fulfillmentChannel: 'Channel A',
      executionStatus: 'Done',
    },
    {
      resourceDetails: 'Laptop 2',
      rrId: 'rr-456',
      resource_request_bug_id: '67890',
      resourceGroups: [],
      resourceRequestTargetDeliveryDate: { year: 2023, month: 2, day: 1 },
      resourceRequestActualDeliveryDate: { year: 2023, month: 2, day: 5 },
      fulfillmentStatus: 1,
      resourceRequestStatus: 1,
      resourceRequestBugStatus: 'Fixed',
      procurementActualDeliveryDate: undefined,
      procurementTargetDeliveryDate: undefined,
      buildActualDeliveryDate: undefined,
      buildTargetDeliveryDate: undefined,
      qaActualDeliveryDate: undefined,
      qaTargetDeliveryDate: undefined,
      configActualDeliveryDate: undefined,
      configTargetDeliveryDate: undefined,
      customer: 'Customer B',
      resourceName: 'Pixel 8',
      acceptedQuantity: 5,
      criticality: 'Low',
      requestApproval: 'Pending',
      resourcePm: 'PM B',
      fulfillmentChannel: 'Channel B',
      executionStatus: 'In Progress',
    },
  ];

  const mockFiltersResponse = {
    rrIds: [],
    resourceDetails: [],
    customer: [],
    resourceName: [],
    criticality: [],
    requestApproval: [],
    resourcePm: [],
    fulfillmentChannel: [],
    executionStatus: [],
    resourceRequestBugStatus: [],
    resourceGroups: [],
  };

  beforeEach(() => {
    (useQuery as jest.Mock).mockImplementation((options) => {
      if (options?.queryKey?.[0] === 'filters') {
        return {
          data: mockFiltersResponse,
          isPending: false,
          isError: false,
        };
      }
      if (options?.queryKey?.[0] === 'list') {
        return {
          data: {
            resourceRequests: mockResourceRequests,
            nextPageToken: 'next-token',
          },
          isPending: false,
          isError: false,
        };
      }
      return { data: undefined, isPending: true };
    });
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders data rows', async () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <ResourceRequestTable />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Laptop 1')).toBeVisible();
    expect(screen.getByText('Laptop 2')).toBeVisible();
  });

  it('renders loading state', async () => {
    (useQuery as jest.Mock).mockReturnValue({
      data: undefined,
      isPending: true,
      isError: false,
    });

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <ResourceRequestTable />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('loading-spinner')).toBeVisible();
  });
});
