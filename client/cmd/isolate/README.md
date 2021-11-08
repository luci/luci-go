# `isolate` user guide

Isolate your test.


## Introduction

-   `isolate` wraps the vast majority of the client side code relating to
    executable isolation.
-   "`isolate help`" gives you all the help you need so only a quick overview is
    given here.


## isolate

-   "`isolate`" wraps usage for tracing, compiling, archiving and even running a
    test isolated locally.
-   Look at the `isolate help` page for more information.
-   `-isolate` is the preferred input format in the Go
    implementation. `-isolated` is supported in only a few subcommands (e.g.
    `isolate archive`).

### Minimal .isolate file

Here is an example of a minimal .isolate file where an additional file is needed
only on Windows and the command there is different:
```
{
  # 1. Global level.
  'variables': {
    'files': [
      '<(PRODUCT_DIR)/foo_unittests<(EXECUTABLE_SUFFIX)',
      # 1. All files in a subdirectory will be included.
      '../test/data/',
    ],
  },

  # 1. Things that are configuration or OS specific.
  'conditions': [
    ['OS=="linux" or OS=="mac"', {
      'variables': {
        'command': [
          '<(PRODUCT_DIR)/foo_unittests<(EXECUTABLE_SUFFIX)',
        ],
      },
    }],

    ['OS=="android"', {
      'variables': {
        'command': [
          'setup_env.py',
          '<(PRODUCT_DIR)/foo_unittests<(EXECUTABLE_SUFFIX)',
        ],
        'files': [
          'setup_env.py',
        ],
      },
    }],
  ],
}
```


* `EXECUTABLE_SUFFIX` variable is automatically set to `".exe"` on Windows and
  empty on the other OSes.
* `PRODUCT_DIR` is variable passed with --path-variable flag.

Working on Chromium? You are not done yet! You need to create a GN target too,
check out
https://chromium.googlesource.com/chromium/src/+/HEAD/docs/workflow/debugging-with-swarming.md


### Useful subcommands

-   "`isolate check`" verifies a `.isolate` file (It no longer produces a
    `.isolated`).
-   "`isolate archive`" does the equivalent of `check`, then archives the
    isolated tree.
-   "`isolate run`" runs the test locally isolated, so you can verify for any
    failure specific to isolated testing.
