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

import { parse as babelParse } from '@babel/parser';
import { NullableMappedPosition } from 'source-map';
import StackFrame from 'stackframe';

import {
  getSourceMapUrl,
  trimLeadingParents,
  formatFrame,
  findEnclosingFunctionName,
} from './utils';

describe('Error Reporting Utils', () => {
  describe('getSourceMapUrl', () => {
    it('should append .map to a clean url', () => {
      const url = 'http://example.com/foo.js';
      expect(getSourceMapUrl(url)).toBe('http://example.com/foo.js.map');
    });

    it('should remove both query parameters and hash fragments', () => {
      const url = 'http://example.com/foo.js?bar=baz#qux';
      expect(getSourceMapUrl(url)).toBe('http://example.com/foo.js.map');
    });
  });

  describe('trimLeadingParents', () => {
    it('should remove leading ../', () => {
      const path = '../../src/components/button.js';
      expect(trimLeadingParents(path)).toBe('src/components/button.js');
    });

    it('should not modify a path with no leading ../', () => {
      const path = 'src/components/button.js';
      expect(trimLeadingParents(path)).toBe('src/components/button.js');
    });
  });

  describe('formatFrame', () => {
    const minifiedFrame = new StackFrame({
      functionName: 'a',
      fileName: 'http://example.com/foo.js',
      lineNumber: 1,
      columnNumber: 1,
    });

    it('should return the minified frame if originalFrame is not provided', () => {
      expect(formatFrame(minifiedFrame)).toBe(
        '    at a (http://example.com/foo.js:1:1)',
      );
    });

    it('should return the minified frame if originalFrame is missing source', () => {
      const originalFrame: NullableMappedPosition = {
        source: null,
        line: 1,
        column: 1,
        name: 'originalFunc',
      };
      expect(formatFrame(minifiedFrame, originalFrame)).toBe(
        '    at a (http://example.com/foo.js:1:1)',
      );
    });

    it('should return the minified frame if originalFrame is missing line', () => {
      const originalFrame: NullableMappedPosition = {
        source: 'src/bar.js',
        line: null,
        column: 1,
        name: 'originalFunc',
      };
      expect(formatFrame(minifiedFrame, originalFrame)).toBe(
        '    at a (http://example.com/foo.js:1:1)',
      );
    });

    it('should return the minified frame if originalFrame is missing column', () => {
      const originalFrame: NullableMappedPosition = {
        source: 'src/bar.js',
        line: 1,
        column: null,
        name: 'originalFunc',
      };
      expect(formatFrame(minifiedFrame, originalFrame)).toBe(
        '    at a (http://example.com/foo.js:1:1)',
      );
    });

    it('should format the frame using the original position', () => {
      const originalFrame: NullableMappedPosition = {
        source: '../../src/bar.js',
        line: 10,
        column: 5,
        name: 'originalFunc',
      };
      expect(formatFrame(minifiedFrame, originalFrame)).toBe(
        '    at originalFunc (src/bar.js:10:5)',
      );
    });

    it('should use the minified function name if the original is missing', () => {
      const originalFrame: NullableMappedPosition = {
        source: '../../src/bar.js',
        line: 10,
        column: 5,
        name: null,
      };
      expect(formatFrame(minifiedFrame, originalFrame)).toBe(
        '    at a (src/bar.js:10:5)',
      );
    });

    it('should use ANONYMOUS_CALLABLE if both names are missing', () => {
      const minifiedFrameWithoutName = new StackFrame({
        ...minifiedFrame,
        functionName: undefined,
      });
      const originalFrame: NullableMappedPosition = {
        source: '../../src/bar.js',
        line: 10,
        column: 5,
        name: null,
      };
      expect(formatFrame(minifiedFrameWithoutName, originalFrame)).toBe(
        `    at <anonymous> (src/bar.js:10:5)`,
      );
    });
  });

  describe('findEnclosingFunctionName', () => {
    const parse = (code: string) =>
      babelParse(code, {
        sourceType: 'module',
        plugins: ['typescript', 'jsx'],
      });

    it('should find a named function declaration', () => {
      const code = `
        function myFunction() {
          console.log('hello');
        }
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 3, 10)).toBe('myFunction');
    });

    it('should find a named function expression', () => {
      const code = `
        const x = function myFunction() {
          console.log('hello');
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 3, 10)).toBe('myFunction');
    });

    it('should find a class method', () => {
      const code = `
        class MyClass {
          myMethod() {
            console.log('hello');
          }
        }
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('myMethod');
    });

    it('should find an object method', () => {
      const code = `
        const myObj = {
          myMethod() {
            console.log('hello');
          }
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('myMethod');
    });

    it('should find an arrow function assigned to a variable', () => {
      const code = `
        const myFunction = () => {
          console.log('hello');
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 3, 10)).toBe('myFunction');
    });

    it('should find a function expression assigned to a variable', () => {
      const code = `
        const myFunction = function() {
          console.log('hello');
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 3, 10)).toBe('myFunction');
    });

    it('should find an arrow function in an object property', () => {
      const code = `
        const myObj = {
          myMethod: () => {
            console.log('hello');
          }
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('myMethod');
    });

    it('should find an arrow function in an object property with string literal key', () => {
      const code = `
        const myObj = {
          "my-method": () => {
            console.log('hello');
          }
        };
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('my-method');
    });

    it('should find an arrow function in a class property', () => {
      const code = `
        class MyClass {
          myMethod = () => {
            console.log('hello');
          };
        }
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('myMethod');
    });

    it('should find an arrow function in a class property with string literal key', () => {
      const code = `
        class MyClass {
          "my-method" = () => {
            console.log('hello');
          };
        }
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('my-method');
    });

    it('should find an inline arrow function in a JSX attribute', () => {
      const code = `
        const MyComponent = () => (
          <div onClick={() => {
            console.log('clicked');
          }}>
            Click me
          </div>
        );
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('onClick');
    });

    it('should return the inner-most function name', () => {
      const code = `
        function outer() {
          function inner() {
            console.log('hello');
          }
        }
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 4, 12)).toBe('inner');
    });

    it('should return null if no enclosing function is found', () => {
      const code = `
        console.log('hello');
      `;
      const ast = parse(code);
      expect(findEnclosingFunctionName(ast, 2, 8)).toBe(null);
    });
  });
});
