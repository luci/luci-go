// Copyright 2023 The LUCI Authors.
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

import { cleanup, render, screen } from '@testing-library/react';

import {
  alreadyCanceledBuild,
  canaryFailedBuild,
  canarySucceededBuild,
  resourceExhaustionBuild,
  scheduledToBeCanceledBuild,
  succeededTimeoutBuild,
  timeoutBuild,
} from '@/build/testing_tools/mock_builds';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildContextProvider } from '../../context';

import { AlertsSection } from './alerts_section';
describe('<AlertsSection />', () => {
  describe('canary warning', () => {
    afterEach(() => {
      cleanup();
    });
    it('should render canary warning correctly', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={canaryFailedBuild}>
            <AlertsSection />
          </BuildContextProvider>
        </FakeContextProvider>,
      );

      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardWarning');
      expect(alert).toHaveTextContent(
        'This build ran on a canary version of LUCI.',
      );
    });

    it('should not render canary warning when the build passed', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={canarySucceededBuild}>
            <AlertsSection />
          </BuildContextProvider>
        </FakeContextProvider>,
      );
      expect(screen.queryByRole('alert')).toBeNull();
    });
  });

  describe('cancel schedule info', () => {
    afterEach(() => {
      cleanup();
    });

    it('should render cancel schedule info correctly', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={scheduledToBeCanceledBuild}>
            <AlertsSection />
          </BuildContextProvider>
        </FakeContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardInfo');
      expect(alert).toHaveTextContent(
        'This build was scheduled to be canceled by user:bb_user@google.com',
      );
    });

    it('should not render cancel schedule info when the build is already canceled', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={alreadyCanceledBuild}>
            <AlertsSection />
          </BuildContextProvider>
          ,
        </FakeContextProvider>,
      );
      expect(screen.queryByRole('alert')).toBeNull();
    });
  });

  describe('status details', () => {
    afterEach(() => {
      cleanup();
    });

    it('should render resource exhaustion error correctly', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={resourceExhaustionBuild}>
            <AlertsSection />
          </BuildContextProvider>
          ,
        </FakeContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardError');
      expect(alert).toHaveTextContent(
        'This build failed due to resource exhaustion',
      );
    });

    it('should render timeout error correctly', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={timeoutBuild}>
            <AlertsSection />
          </BuildContextProvider>
        </FakeContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardError');
      expect(alert).toHaveTextContent('This build timed out');
    });

    // This may happen when the timeout build finishes during the grace period.
    it('should not render timeout error when the build succeeded', () => {
      render(
        <FakeContextProvider>
          <BuildContextProvider build={succeededTimeoutBuild}>
            <AlertsSection />
          </BuildContextProvider>
        </FakeContextProvider>,
      );
      expect(screen.queryByRole('alert')).toBeNull();
    });
  });
});
