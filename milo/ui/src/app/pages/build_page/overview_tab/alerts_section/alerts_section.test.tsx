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

import { Build, BuildbucketStatus } from '@/common/services/buildbucket';

import { BuildContextProvider } from '../../context';

import { AlertsSection } from './alerts_section';

const canaryFailedBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.InfraFailure,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  input: {
    experiments: ['luci.buildbucket.canary_software'],
  },
};

const canarySucceededBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Success,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  input: {
    experiments: ['luci.buildbucket.canary_software'],
  },
};

const scheduledToBeCanceledBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Started,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  cancelTime: '2020-12-12T02:01:01',
  canceledBy: 'user:bb_user@google.com',
  gracePeriod: '30s',
};

const alreadyCanceledBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Canceled,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  cancelTime: '2020-12-12T02:01:01',
  canceledBy: 'user:bb_user@google.com',
  gracePeriod: '30s',
};

const resourceExhaustionBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Failure,
  statusDetails: {
    resourceExhaustion: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
};

const timeoutBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Failure,
  statusDetails: {
    timeout: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
};

const succeededTimeoutBuild: Build = {
  id: '1234',
  status: BuildbucketStatus.Success,
  statusDetails: {
    timeout: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
};

describe('AlertsSection', () => {
  describe('canary warning', () => {
    afterEach(() => {
      cleanup();
    });

    it('should render canary warning correctly', () => {
      render(
        <BuildContextProvider build={canaryFailedBuild}>
          <AlertsSection />
        </BuildContextProvider>,
      );

      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardWarning');
      expect(alert).toHaveTextContent(
        'This build ran on a canary version of LUCI.',
      );
    });

    it('should not render canary warning when the build passed', () => {
      render(
        <BuildContextProvider build={canarySucceededBuild}>
          <AlertsSection />
        </BuildContextProvider>,
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
        <BuildContextProvider build={scheduledToBeCanceledBuild}>
          <AlertsSection />
        </BuildContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardInfo');
      expect(alert).toHaveTextContent(
        'This build was scheduled to be canceled by user:bb_user@google.com',
      );
    });

    it('should not render cancel schedule info when the build is already canceled', () => {
      render(
        <BuildContextProvider build={alreadyCanceledBuild}>
          <AlertsSection />
        </BuildContextProvider>,
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
        <BuildContextProvider build={resourceExhaustionBuild}>
          <AlertsSection />
        </BuildContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardError');
      expect(alert).toHaveTextContent(
        'This build failed due to resource exhaustion',
      );
    });

    it('should render timeout error correctly', () => {
      render(
        <BuildContextProvider build={timeoutBuild}>
          <AlertsSection />
        </BuildContextProvider>,
      );
      const alert = screen.getByRole('alert');
      expect(alert).toHaveClass('MuiAlert-standardError');
      expect(alert).toHaveTextContent('This build timed out');
    });

    // This may happen when the timeout build finishes during the grace period.
    it('should not render timeout error when the build succeeded', () => {
      render(
        <BuildContextProvider build={succeededTimeoutBuild}>
          <AlertsSection />
        </BuildContextProvider>,
      );
      expect(screen.queryByRole('alert')).toBeNull();
    });
  });
});
