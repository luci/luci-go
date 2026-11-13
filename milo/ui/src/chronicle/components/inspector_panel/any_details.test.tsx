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

import { AnyDetails } from './any_details';

const TEST_SUMMARY_TYPE_URL =
  'type.googleapis.com/turboci.data.test.v1.TestCheckSummaryResult';
const BUILD_RESULTS_TYPE_URL =
  'type.googleapis.com/turboci.data.build.v1.BuildCheckResults';
const STAGE_ATTEMPT_DETAILS_TYPE_URL =
  'type.googleapis.com/turboci.data.stage.v1.CommonStageAttemptDetails';

describe('AnyDetails', () => {
  describe.each([
    {
      name: 'TestCheckSummaryResult.view_url',
      typeUrl: TEST_SUMMARY_TYPE_URL,
      toJson: (url: string) => JSON.stringify({ viewUrl: url }),
    },
    {
      name: 'BuildCheckResults.view_url',
      typeUrl: BUILD_RESULTS_TYPE_URL,
      toJson: (url: string) => JSON.stringify({ viewUrl: url }),
    },
    {
      name: 'CommonStageAttemptDetails.view_urls',
      typeUrl: STAGE_ATTEMPT_DETAILS_TYPE_URL,
      toJson: (url: string) => JSON.stringify({ viewUrls: { Logs: { url } } }),
    },
  ])('$name', ({ typeUrl, toJson }) => {
    it('renders an http(s) view URL as a link', () => {
      const url = 'https://ci.chromium.org/b/123';
      render(<AnyDetails typeUrl={typeUrl} json={toJson(url)} />);

      expect(screen.getByRole('link', { name: url })).toHaveAttribute(
        'href',
        url,
      );
    });

    it('renders a non-http(s) view URL as plain text', () => {
      const url = 'javascript:alert(1)';
      const { container } = render(
        <AnyDetails typeUrl={typeUrl} json={toJson(url)} />,
      );

      expect(screen.queryByRole('link')).not.toBeInTheDocument();
      expect(container).toHaveTextContent(url);
    });
  });

  it.each(['toString', 'constructor', '__proto__'])(
    'falls back to the JSON view when the type URL is %s',
    (typeUrl) => {
      const { container } = render(
        <AnyDetails typeUrl={typeUrl} json={JSON.stringify({ a: 1 })} />,
      );

      expect(screen.getByText(typeUrl)).toBeInTheDocument();
      expect(container).toHaveTextContent('"a": 1');
    },
  );
});
