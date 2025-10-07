import { fireEvent, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

import { OutputTestVerdict } from '@/common/types/verdict';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { OverviewActionsSection } from './overview_actions_section'; // Assuming the component is in this path

const MOCK_PROJECT_ID = 'test-project';
const MOCK_TEST_ID = 'test/id/some.Test';
const MOCK_RAW_INVOCATION_ID = 'inv-id-123';

describe('<OverviewActionsSection />', () => {
  let mockInvocation: Invocation;
  let mockTestVariant: TestVariant;

  beforeEach(() => {
    mockInvocation = Invocation.fromPartial({
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
      name: 'invocations/ants-build-12345',
    });
    mockTestVariant = TestVariant.fromPartial({
      testId: MOCK_TEST_ID,
      testIdStructured: {
        coarseName: 'coarse',
        fineName: 'fine',
        moduleName: 'module',
        caseName: 'case',
      },
      variant: {
        def: {
          builder: 'android-builder',
          target: 'generic_x86',
          branch: 'some_branch',
        },
      },
      results: [{ result: { tags: [] } }],
    });
  });

  const renderComponent = (
    customInvocation?: Invocation,
    customTestVariant?: TestVariant,
  ) => {
    const inv = customInvocation || mockInvocation;
    const tv = customTestVariant || mockTestVariant;
    return render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={inv}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation
        >
          <TestVariantProvider
            testVariant={tv as OutputTestVerdict}
            displayStatusString="failed"
          >
            <OverviewActionsSection />
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('should render the "Rerun" button with dropdown icon when not android invocation', () => {
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      name: 'build-12345',
    });

    renderComponent(currentInv, undefined);

    const rerunButton = screen.getByTestId('rerun-button');
    expect(rerunButton).toBeInTheDocument();

    const rerunDropdown = screen.queryByTestId('rerun-dropdown');
    expect(rerunDropdown).not.toBeInTheDocument();
    expect(
      screen.queryByText('Rerun test case with atest'),
    ).not.toBeInTheDocument();
  });

  it('should show run test locally on invalid invocation name', () => {
    renderComponent(undefined, undefined);

    const rerunButton = screen.queryByTestId('rerun-button');
    expect(rerunButton).not.toBeInTheDocument();

    expect(screen.getByText('Rerun test case locally')).toBeInTheDocument();
  });

  it('should render dropdown rerun button for android invocations', () => {
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      tags: [{ key: 'local_test_runner', value: 'atest' }],
    });
    renderComponent(currentInv, undefined);

    const rerunButton = screen.queryByTestId('rerun-button');
    expect(rerunButton).not.toBeInTheDocument();

    const rerunDropdown = screen.getByTestId('rerun-dropdown');
    expect(rerunDropdown).toBeInTheDocument();
  });

  it('should show atest and module rerun command when dropdown menu is selected with valid invocation name', () => {
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      tags: [{ key: 'local_test_runner', value: 'atest' }],
    });
    renderComponent(currentInv, undefined);

    const rerunDropdown = screen.getByTestId('rerun-dropdown');
    fireEvent.click(rerunDropdown);

    expect(screen.getByText('Rerun test case with atest')).toBeInTheDocument();
    expect(
      screen.getByText('Rerun full module with atest'),
    ).toBeInTheDocument();
  });
});
