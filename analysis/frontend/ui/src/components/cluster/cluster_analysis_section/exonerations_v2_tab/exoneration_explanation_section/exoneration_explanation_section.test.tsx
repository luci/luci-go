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

import '@testing-library/jest-dom';

import {
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';

import { ExoneratedTestVariantBranchBuilder, TestCriteria } from '../model/mocks';
import ExonerationExplanationSection from './exoneration_explanation_section';

describe('Test ExonerationExplanationSection', () => {
  it('shows statistics related to test flakiness', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    // Flaky criteria section.
    expect(await screen.findByTestId('flaky_criteria_met_chip')).toHaveTextContent('Not met');

    expect(screen.getByText('111111111111')).toBeInTheDocument();
    expect(screen.getByText('changelist-one #8111')).toBeInTheDocument();

    // Failure criteria section.
    expect(await screen.findByTestId('failure_criteria_met_chip')).toHaveTextContent('Not met');

    expect(screen.getByText('333333333333')).toBeInTheDocument();
    expect(screen.getByText('changelist-three #8333')).toBeInTheDocument();
  });

  it('expanding and collapsing flaky verdicts criteria works', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    // As the criteria was not met, it should not have been auto-expanded by default.
    expect(screen.getByTestId('flaky_criteria_details')).not.toBeVisible();

    // Expand the section.
    fireEvent.click(screen.getByTestId('flaky_criteria_header'));
    await waitFor(() => {
      expect(screen.getByTestId('flaky_criteria_details')).toBeVisible();
    });

    // Collapse the section.
    fireEvent.click(screen.getByTestId('flaky_criteria_header'));
    await waitFor(() => {
      expect(screen.getByTestId('flaky_criteria_details')).not.toBeVisible();
    });
  });

  it('expanding and collapsing failure criteria section works', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    // As the criteria was not met, it should not have been auto-expanded by default.
    expect(screen.getByTestId('failure_criteria_details')).not.toBeVisible();

    // Expand the section.
    fireEvent.click(screen.getByTestId('failure_criteria_header'));
    await waitFor(() => {
      expect(screen.getByTestId('failure_criteria_details')).toBeVisible();
    });

    // Collapse the section.
    fireEvent.click(screen.getByTestId('failure_criteria_header'));
    await waitFor(() => {
      expect(screen.getByTestId('failure_criteria_details')).not.toBeVisible();
    });
  });

  it('if flaky verdicts criteria almost met, section is auto-expanded', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder()
        .almostMeetsFlakyThreshold().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    expect(await screen.findByTestId('flaky_criteria_met_chip')).toHaveTextContent('Almost met');

    // The flaky verdicts criteria section should have auto-expanded.
    expect(screen.getByTestId('flaky_criteria_details')).toBeVisible();
  });

  it('if flaky verdicts criteria met, section is auto-expanded', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder()
        .meetsFlakyThreshold().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    expect(await screen.findByTestId('flaky_criteria_met_chip')).toHaveTextContent('Met');

    // The flaky verdicts criteria section should have auto-expanded.
    expect(screen.getByTestId('flaky_criteria_details')).toBeVisible();
  });

  it('if recent failing verdicts criteria almost met, the section is auto-expanded', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder()
        .almostMeetsFailureThreshold().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    expect(await screen.findByTestId('failure_criteria_met_chip')).toHaveTextContent('Almost met');

    // The recently failing verdicts criteria section should have auto-expanded.
    expect(screen.getByTestId('failure_criteria_details')).toBeVisible();
  });

  it('if recent failing verdicts criteria met, the section is auto-expanded', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder()
        .meetsFailureThreshold().build();
    render(
        <ExonerationExplanationSection
          project='testproject'
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    expect(await screen.findByTestId('failure_criteria_met_chip')).toHaveTextContent('Met');

    // The recently failing verdicts criteria section should have auto-expanded.
    expect(screen.getByTestId('failure_criteria_details')).toBeVisible();
  });
});
