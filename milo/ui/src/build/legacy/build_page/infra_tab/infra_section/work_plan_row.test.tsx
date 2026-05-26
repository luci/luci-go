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

import { cleanup, render, screen } from '@testing-library/react';

import { BuildInfra_TurboCI } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';

import { WorkPlanRow } from './work_plan_row';

describe('<WorkPlanRow />', () => {
  afterEach(() => {
    cleanup();
  });

  it('should render "None" if turboci is not provided', () => {
    render(
      <table>
        <tbody>
          <WorkPlanRow />
        </tbody>
      </table>,
    );
    expect(screen.getByText('WorkPlan:')).toBeInTheDocument();
    expect(screen.getByText('None')).toBeInTheDocument();
    expect(screen.queryByRole('link')).toBeNull();
  });

  it('should render "None" if stageAttemptId is not provided', () => {
    const turboci = BuildInfra_TurboCI.fromPartial({});
    render(
      <table>
        <tbody>
          <WorkPlanRow turboci={turboci} />
        </tbody>
      </table>,
    );
    expect(screen.getByText('WorkPlan:')).toBeInTheDocument();
    expect(screen.getByText('None')).toBeInTheDocument();
    expect(screen.queryByRole('link')).toBeNull();
  });

  it('should render "None" if stageAttemptId parsing fails', () => {
    const turboci = BuildInfra_TurboCI.fromPartial({
      stageAttemptId: 'invalid_format',
    });
    render(
      <table>
        <tbody>
          <WorkPlanRow turboci={turboci} />
        </tbody>
      </table>,
    );
    expect(screen.getByText('WorkPlan:')).toBeInTheDocument();
    expect(screen.getByText('None')).toBeInTheDocument();
    expect(screen.queryByRole('link')).toBeNull();
  });

  it('should render the chronicle link with the stageAttemptId dropping the last colon and suffix', () => {
    const turboci = BuildInfra_TurboCI.fromPartial({
      stageAttemptId: 'L54321:S12345:A1',
    });
    render(
      <table>
        <tbody>
          <WorkPlanRow turboci={turboci} />
        </tbody>
      </table>,
    );

    expect(screen.getByText('WorkPlan:')).toBeInTheDocument();
    const link = screen.getByRole('link');
    expect(link).toBeInTheDocument();
    expect(link).toHaveTextContent('L54321:S12345');
    expect(link).toHaveAttribute(
      'href',
      '/ui/chronicle/54321/graph?nodeId=12345',
    );
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute(
      'aria-label',
      'graph view for attempt 1 of stage 12345 in workplan 54321',
    );
  });

  it('should handle other stageAttemptId formats by dropping the last colon and suffix', () => {
    const turboci = BuildInfra_TurboCI.fromPartial({
      stageAttemptId: 'L54321:S09876:A10',
    });
    render(
      <table>
        <tbody>
          <WorkPlanRow turboci={turboci} />
        </tbody>
      </table>,
    );

    expect(screen.getByText('WorkPlan:')).toBeInTheDocument();
    const link = screen.getByRole('link');
    expect(link).toBeInTheDocument();
    expect(link).toHaveTextContent('L54321:S09876');
    expect(link).toHaveAttribute(
      'aria-label',
      'graph view for attempt 10 of stage 09876 in workplan 54321',
    );
  });
});
