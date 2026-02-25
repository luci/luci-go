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

import { Navigate } from 'react-router';

import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';

interface BaseDeviceRedirectProps {
  isLoading: boolean;
  error: Error | null;
  devices?: readonly { id: string }[];
  generateUrl: (id: string) => string;
  testId: string;
  filter: string;
}

export function BaseDeviceRedirect({
  isLoading,
  error,
  devices,
  generateUrl,
  testId,
  filter,
}: BaseDeviceRedirectProps) {
  // Intentionally show a blank page when loading. Showing a loading bar is
  // more jarring because the load time is quick for one device.
  if (isLoading) return <></>;

  if (error) {
    return (
      <AlertWithFeedback
        testId={testId}
        severity="error"
        title="Redirection failed"
        bugErrorMessage={`Device not found for query: ${filter}`}
      >
        <p>An error occured.</p>
      </AlertWithFeedback>
    );
  }

  if (!devices?.length) {
    return (
      <AlertWithFeedback
        testId={testId}
        severity="error"
        title="No devices found"
        bugErrorMessage={`Device not found for query: ${filter}`}
      >
        <p>
          No devices matched the search, so the redirection was canceled. It
          {"'"}s possible the link you clicked might be malformed.
        </p>
      </AlertWithFeedback>
    );
  }

  return <Navigate to={generateUrl(devices[0].id)} />;
}
