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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { StrictMode, useState } from 'react';

import { SingletonStoreProvider } from './context';
import { useSingleton } from './use_singleton';

interface TestComponentProps<T> {
  readonly factory: (
    numParam: number,
    strParam: string,
    objParam: Record<never, never> | undefined,
    ...rest: string[]
  ) => T;
  readonly numParam1?: number;
  readonly numParam2?: number;
  readonly strParam?: string;
  readonly objParam?: Record<never, never>;
  readonly restParam?: readonly string[];
  readonly callback: (value: T) => void;
}

const STABLE_OBJECT = Object.freeze({});

function TestComponent<T>({
  factory,
  numParam1 = 0,
  numParam2 = 0,
  strParam = '',
  objParam = STABLE_OBJECT,
  restParam,
  callback,
}: TestComponentProps<T>) {
  const [innerState, setInnerState] = useState(0);
  const value = useSingleton({
    key: [
      numParam1 + numParam2 + innerState,
      strParam,
      objParam,
      ...(restParam || []),
    ],
    fn: () =>
      factory(
        numParam1 + numParam2 + innerState,
        strParam,
        objParam,
        ...(restParam || []),
      ),
  });
  callback(value);
  return <button onClick={() => setInnerState(innerState + 1)}>button</button>;
}

describe('useSingleton', () => {
  let factory: jest.Mock<unknown>;

  beforeEach(() => {
    let callCount = 0;
    factory = jest.fn((..._params) => {
      callCount += 1;
      return { [callCount]: callCount };
    });
  });

  afterEach(() => {
    cleanup();
    factory.mockRestore();
  });

  it('should only create instance once when the same factory + params are requested', () => {
    const callback1 = jest.fn();
    const callback2 = jest.fn();
    render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={1}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={1}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance = callback1.mock.calls[0][0];
    expect(callback1).toHaveBeenCalledTimes(1);
    expect(callback1).toHaveBeenLastCalledWith(instance);
    expect(callback2).toHaveBeenCalledTimes(1);
    expect(callback2).toHaveBeenLastCalledWith(instance);
  });

  it('should return the same instance when the computed params are the same', () => {
    const callback1 = jest.fn();
    const callback2 = jest.fn();
    render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={1}
          numParam2={4}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={2}
          numParam2={3}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance = factory.mock.results[0].value;
    expect(callback1).toHaveBeenCalledTimes(1);
    expect(callback1).toHaveBeenLastCalledWith(instance);
    expect(callback2).toHaveBeenCalledTimes(1);
    expect(callback2).toHaveBeenLastCalledWith(instance);
  });

  it('should work when multiple params sets are requested', () => {
    const callback1 = jest.fn();
    const callback2 = jest.fn();
    const callback3 = jest.fn();
    const callback4 = jest.fn();
    render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="different-string"
          callback={callback2}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback3}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="different-string"
          callback={callback4}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    const instance1 = callback1.mock.calls[0][0];
    const instance2 = callback2.mock.calls[0][0];
    expect(instance1).not.toBe(instance2);
    expect(callback1).toHaveBeenCalledTimes(1);
    expect(callback1).toHaveBeenLastCalledWith(instance1);
    expect(callback2).toHaveBeenCalledTimes(1);
    expect(callback2).toHaveBeenLastCalledWith(instance2);
    expect(callback3).toHaveBeenCalledTimes(1);
    expect(callback3).toHaveBeenLastCalledWith(instance1);
    expect(callback4).toHaveBeenCalledTimes(1);
    expect(callback4).toHaveBeenLastCalledWith(instance2);
  });

  it('should preserve the instance through rerenders', () => {
    const callback = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance = factory.mock.results[0].value;
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenLastCalledWith(instance);

    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(callback).toHaveBeenCalledTimes(2);
    expect(callback).toHaveBeenLastCalledWith(instance);
  });

  it('should create a new instance when params are different', () => {
    const callback = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={2}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance1 = factory.mock.results[0].value;
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenLastCalledWith(instance1);

    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    const instance2 = factory.mock.results[1].value;
    expect(instance2).not.toBe(instance1);
    expect(callback).toHaveBeenCalledTimes(2);
    expect(callback).toHaveBeenLastCalledWith(instance2);
  });

  it('should not create a new instance when param are only referentially different', () => {
    const callback = jest.fn();
    const objParam1 = {};
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          objParam={objParam1}
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance1 = factory.mock.results[0].value;
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenLastCalledWith(instance1);

    const objParam2 = {};
    expect(objParam2).toStrictEqual(objParam1);
    expect(objParam2).not.toBe(objParam1);
    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          objParam={objParam2}
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledTimes(2);
    expect(callback).toHaveBeenLastCalledWith(instance1);
  });

  it('can handle new component', () => {
    const callback1 = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance = factory.mock.results[0].value;
    expect(callback1).toHaveBeenCalledTimes(1);
    expect(callback1).toHaveBeenLastCalledWith(instance);

    const callback2 = jest.fn();
    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(callback1).toHaveBeenCalledTimes(2);
    expect(callback1).toHaveBeenLastCalledWith(instance);
    expect(callback2).toHaveBeenCalledTimes(1);
    expect(callback2).toHaveBeenLastCalledWith(instance);
  });

  it('can handle removed component', () => {
    const callback1 = jest.fn();
    const callback2 = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance = factory.mock.results[0].value;
    expect(callback1).toHaveBeenCalledTimes(1);
    expect(callback1).toHaveBeenLastCalledWith(instance);
    expect(callback2).toHaveBeenCalledTimes(1);
    expect(callback2).toHaveBeenLastCalledWith(instance);

    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
      </SingletonStoreProvider>,
    );
    expect(callback1).toHaveBeenCalledTimes(2);
    expect(callback1).toHaveBeenLastCalledWith(instance);
    expect(callback2).toHaveBeenCalledTimes(1);
  });

  it('can handle param count change', () => {
    const callback = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          restParam={['string1', 'string2']}
          factory={factory}
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance1 = factory.mock.results[0].value;
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenLastCalledWith(instance1);

    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          restParam={['string1', 'string2', 'string3']}
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    const instance2 = factory.mock.results[1].value;
    expect(instance2).not.toBe(instance1);
    expect(callback).toHaveBeenCalledTimes(2);
    expect(callback).toHaveBeenLastCalledWith(instance2);
  });

  it('unmounting the last subscriber discards the old instance', () => {
    const callback = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={2}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance1 = factory.mock.results[0].value;
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenLastCalledWith(instance1);

    rerender(
      <SingletonStoreProvider>
        <></>
      </SingletonStoreProvider>,
    );
    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={2}
          strParam="string"
          callback={callback}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    const instance2 = factory.mock.results[1].value;
    expect(instance2).not.toBe(instance1);
    expect(callback).toHaveBeenCalledTimes(2);
    expect(callback).toHaveBeenLastCalledWith(instance2);
  });

  it('can handle swap components', () => {
    const callback1 = jest.fn();
    const callback2 = jest.fn();
    const { rerender } = render(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={2}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    const instance1 = factory.mock.results[0].value;
    const instance2 = factory.mock.results[1].value;
    expect(instance1).not.toBe(instance2);

    rerender(
      <SingletonStoreProvider>
        <TestComponent
          factory={factory}
          numParam1={3}
          strParam="string"
          callback={callback1}
        />
        <TestComponent
          factory={factory}
          numParam1={2}
          strParam="string"
          callback={callback2}
        />
      </SingletonStoreProvider>,
    );
    expect(factory).toHaveBeenCalledTimes(2);
    expect(callback1).toHaveBeenCalledTimes(2);
    expect(callback1).toHaveBeenLastCalledWith(instance2);
    expect(callback2).toHaveBeenCalledTimes(2);
    expect(callback2).toHaveBeenLastCalledWith(instance1);
  });

  it('can handle updates in strict mode', () => {
    const callback = jest.fn();
    render(
      <StrictMode>
        <SingletonStoreProvider>
          <TestComponent
            factory={factory}
            numParam1={2}
            strParam="string"
            callback={callback}
          />
        </SingletonStoreProvider>
      </StrictMode>,
      {},
    );
    expect(factory).toHaveBeenCalledTimes(1);
    const instance1 = factory.mock.results[0].value;

    fireEvent.click(screen.getByRole('button'));
    expect(factory).toHaveBeenCalledTimes(2);
    const instance2 = factory.mock.results[1].value;
    expect(instance1).not.toBe(instance2);
    expect(callback).toHaveBeenCalledTimes(4);
    expect(callback).toHaveBeenNthCalledWith(3, instance2);
    expect(callback).toHaveBeenNthCalledWith(4, instance2);

    fireEvent.click(screen.getByRole('button'));
    expect(factory).toHaveBeenCalledTimes(3);
    const instance3 = factory.mock.results[2].value;
    expect(instance2).not.toBe(instance3);
    expect(callback).toHaveBeenCalledTimes(6);
    expect(callback).toHaveBeenNthCalledWith(5, instance3);
    expect(callback).toHaveBeenNthCalledWith(6, instance3);
  });
});
