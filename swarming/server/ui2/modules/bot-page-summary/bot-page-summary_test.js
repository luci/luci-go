// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./bot-page-summary";
import { customMatchers } from "../test_util";
import { prettifyName } from "./bot-page-summary";

describe("bot-page-summary", function () {
  // A reusable HTML element in which we create our element under test.
  const container = document.createElement("div");
  document.body.appendChild(container);

  beforeEach(function () {
    jasmine.addMatchers(customMatchers);
  });

  afterEach(function () {
    container.innerHTML = "";
  });

  // ===============TESTS START====================================

  it("make task names less unique", function () {
    const testCases = [
      {
        input:
          "Perf-Win10-Clang-Golo-GPU-QuadroP400-x86_64-Debug-All-ANGLE (debug)",
        output: "Perf-Win10-Clang-Golo-GPU-QuadroP400-x86_64-Debug-All-ANGLE",
      },
    ];
    for (const test of testCases) {
      expect(prettifyName(test.input)).toEqual(test.output);
    }
  });
});
