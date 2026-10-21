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

import { render, screen } from '@testing-library/react';

import { AuthState } from '@/common/api/auth_state';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import * as UseAndroidRepairQueueModule from '@/fleet/pages/android/repair_queue/use_android_repair_queue';
import {
  AndroidMachineType,
  AndroidRepairQueueItem,
  AndroidWorkspace,
  ListAndroidRepairQueueRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { AndroidMyRepairTasksActions } from './android_my_repair_tasks_actions';

const ME = 'tech@example.com';

const item = (
  deviceId: string,
  fcMachineType: AndroidMachineType,
  claimedBy: string,
  taskId: string,
): AndroidRepairQueueItem =>
  AndroidRepairQueueItem.fromPartial({
    deviceId,
    fcMachineType,
    claimedBy,
    taskId,
  });

const DEVICE = AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE;
const HOST = AndroidMachineType.ANDROID_MACHINE_TYPE_HOST;

const mockQueue = (items: readonly AndroidRepairQueueItem[]) =>
  jest
    .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
    .mockImplementation(
      (_request, options) =>
        ({
          data:
            options?.enabled === false
              ? undefined
              : {
                  repairQueueItems: items,
                  totalSize: items.length,
                  nextPageToken: '',
                  inProgressCount: items.length,
                },
          inProgressCount: options?.enabled === false ? 0 : items.length,
        }) as unknown as ReturnType<
          typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
        >,
    );

const renderActions = (
  authState: AuthState = { identity: `user:${ME}`, email: ME },
) =>
  render(
    <FakeAuthStateProvider value={authState}>
      <AndroidMyRepairTasksActions workspace="Pixel" />
    </FakeAuthStateProvider>,
  );

const fleetConsoleButton = () =>
  screen.getByRole('link', { name: /show devices/i });

describe('<AndroidMyRepairTasksActions />', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('requests the tasks claimed by the current user in the workspace', () => {
    const spy = mockQueue([]);
    renderActions();

    expect(spy).toHaveBeenCalledWith(
      ListAndroidRepairQueueRequest.fromPartial({
        pageSize: 100,
        pageToken: '',
        orderBy: '',
        filter: `claimed_by = "${ME}"`,
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
      }),
      { enabled: true },
    );
  });

  it('renders nothing when the user has no claimed task', () => {
    mockQueue([]);
    const { container } = renderActions();

    expect(container).toBeEmptyDOMElement();
  });

  it('disables the query and renders nothing when the user is unknown', () => {
    const spy = mockQueue([item('d1', DEVICE, 'current_user', '1')]);
    const { container } = renderActions({ identity: '' });

    expect(spy).toHaveBeenCalledWith(expect.anything(), { enabled: false });
    expect(container).toBeEmptyDOMElement();
  });

  it('links a single claimed device to the filtered device list', () => {
    mockQueue([item('d1', DEVICE, ME, '1')]);
    renderActions();

    expect(screen.getByText('My tasks (1):')).toBeInTheDocument();
    const fcHref = fleetConsoleButton().getAttribute('href')!;
    expect(fcHref.startsWith('/ui/fleet/p/pixel/devices?')).toBe(true);
    const filter = new URL(fcHref, 'https://example.com').searchParams.get(
      FILTERS_PARAM_KEY,
    );
    expect(filter).toContain('"d1"');
    expect(fleetConsoleButton()).toHaveAttribute('target', '_blank');
  });

  it('links every claimed task when there are several', () => {
    mockQueue([
      item('d1', DEVICE, ME, '1'),
      item('shared', HOST, ME, '2'),
      item('shared', DEVICE, ME, '3'),
    ]);
    renderActions();

    expect(screen.getByText('My tasks (3):')).toBeInTheDocument();
    const filter = new URL(
      fleetConsoleButton().getAttribute('href')!,
      'https://example.com',
    ).searchParams.get(FILTERS_PARAM_KEY);
    expect(filter).toContain('"d1"');
    expect(filter).toContain('"shared"');
  });
});
