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

import { act, render, screen } from '@testing-library/react';

import { PERM_BUILDS_ADD } from '@/build/constants';
import {
  failedBuild,
  runningBuild,
  scheduledToBeCanceledBuild,
  unretriableBuild,
} from '@/build/testing_tools/mock_builds';
import {
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildContextProvider } from '../../context';

import { ActionButton } from './action_button';

describe('<ActionButton />', () => {
  describe('when build ended', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
      jest.restoreAllMocks();
    });

    it('should show disabled retry button when the user has no permission', async () => {
      const openDialogSpy = jest.fn();

      render(
        <FakeContextProvider>
          <BuildContextProvider build={failedBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Retry Build');
      expect(button).toBeDisabled();
    });

    it('should show enabled retry button when the user has permission', async () => {
      const openDialogSpy = jest.fn();
      const batchCheckPermissionsMock = jest
        .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
        .mockImplementation(async (req) =>
          BatchCheckPermissionsResponse.fromPartial({
            results: Object.fromEntries(req.permissions.map((p) => [p, true])),
          }),
        );

      render(
        <FakeContextProvider>
          <BuildContextProvider build={failedBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());
      expect(batchCheckPermissionsMock).toHaveBeenCalledWith(
        expect.objectContaining({
          realm: `${failedBuild.builder.project}:${failedBuild.builder.bucket}`,
          permissions: expect.arrayContaining([PERM_BUILDS_ADD]),
        }),
      );

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Retry Build');
      expect(button).not.toBeDisabled();
    });

    it('should show disabled retry button when the build cannot be retried', async () => {
      const openDialogSpy = jest.fn();
      jest
        .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
        .mockImplementation(async (req) =>
          BatchCheckPermissionsResponse.fromPartial({
            results: Object.fromEntries(req.permissions.map((p) => [p, true])),
          }),
        );

      render(
        <FakeContextProvider>
          <BuildContextProvider build={unretriableBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Retry Build');
      expect(button).toBeDisabled();
    });
  });

  describe('when build has not ended', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
      jest.restoreAllMocks();
    });

    it('should show disabled cancel button when the user has no permission', async () => {
      const openDialogSpy = jest.fn();

      render(
        <FakeContextProvider>
          <BuildContextProvider build={runningBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Cancel Build');
      expect(button).toBeDisabled();
    });

    it('should show enabled cancel button when the user has permission', async () => {
      const openDialogSpy = jest.fn();
      const batchCheckPermissionsMock = jest
        .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
        .mockImplementation(async (req) =>
          BatchCheckPermissionsResponse.fromPartial({
            results: Object.fromEntries(req.permissions.map((p) => [p, true])),
          }),
        );

      render(
        <FakeContextProvider>
          <BuildContextProvider build={runningBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());
      expect(batchCheckPermissionsMock).toHaveBeenCalledWith(
        expect.objectContaining({
          realm: `${failedBuild.builder.project}:${failedBuild.builder.bucket}`,
          permissions: expect.arrayContaining([PERM_BUILDS_ADD]),
        }),
      );

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Cancel Build');
      expect(button).not.toBeDisabled();
    });

    it('should show disabled cancel button when the build is already scheduled to be canceled', async () => {
      const openDialogSpy = jest.fn();
      jest
        .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
        .mockImplementation(async (req) =>
          BatchCheckPermissionsResponse.fromPartial({
            results: Object.fromEntries(req.permissions.map((p) => [p, true])),
          }),
        );

      render(
        <FakeContextProvider>
          <BuildContextProvider build={scheduledToBeCanceledBuild}>
            <ActionButton openDialog={openDialogSpy} />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      await act(() => jest.runAllTimersAsync());

      const button = screen.getByRole('button');
      expect(button).toHaveTextContent('Cancel Build');
      expect(button).toBeDisabled();
    });
  });
});
