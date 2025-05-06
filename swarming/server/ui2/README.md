# Web Components Swarming UI


This is the Web Components based UI. It aims to be lighter-weight and more
future-proof than the previous Polymer v1 UI, but functionally identical.

## Prerequisites

You will install a node.js, npm and npx via cipd (they usually come together).
You should always use the same node/npm/npx used in bots. Please use
`gclient sync` to fetch them.

## npm install

After doing a git pull, before running any make commands, you must run:

    make install_deps

This should only be required once per pull (in case package.json was updated).

## Building

To build the pages for deploying, run:

    make release

The output of the dist/ folder will have the bundled HTML, JS and CSS files.
This should be checked in so as to fit with the App Engine deployment setup.

## Using the demo pages

To build the pages locally for demoing/developing, run:

    make serve

Then, navigate to <http://localhost:8080/newres/swarming-index.html> to see
one of the demo pages.  You can navigate to newres/[foo] where foo is one
of the modules (found in ./modules/) or one of the top level HTML files
(found in ./pages/). The pages in ./modules have mock data, so those are
generally more useful.

If you are running `make serve` on a cloudtop you can access the demo pages on your local machine by using an ssh-tunnel. Run the command

    ssh <your-user-name>@<your-cloudtop-host> -L 8080:localhost:8080


All the links should work as expected if opened on local machine.

The list of all demo pages so far (for easy clicking):

  - [bot-list](http://localhost:8080/newres/bot-list.html)
  - [bot-mass-delete](http://localhost:8080/newres/bot-mass-delete.html)
  - [sort-toggle](http://localhost:8080/newres/sort-toggle.html)
  - [swarming-app](http://localhost:8080/newres/swarming-app.html)
  - [swarming-index](http://localhost:8080/newres/swarming-index.html)
  - [task-list](http://localhost:8080/newres/task-list.html)
  - [task-mass-cancel](http://localhost:8080/newres/task-mass-cancel.html)
  - [task-page](http://localhost:8080/newres/task-page.html)

The login is mocked so it works w/o an internet connection.

It's also possible to test using a "live_demo" mode which will pull actual
data in from `chromium-swarm-dev.appspot.com`.

The are a few steps to enable this. This assumes `depot_tools` are installed.

1. Run `luci-auth login`
2. Run `make live_demo`
3. Navigate to the demo pages normally. They will now use data from the development environment.


## Running the tests

Any file matching `modules/**/*_test.js` will automatically be added to the
test suite. When developing tests, it is easiest to put the tests in
"automatically rebuild and run" mode, which can be done with
`make continuous_test`.

To run all tests exactly once on Firefox and Chrome (assuming those browsers
are present):

    make test

To ease this a little bit, many assertions have been modified by adding an
(undocumented) second param. This second param will be shown if the assertion
fails and can make it a little easier to track down. For example:
`expect(...).toContain('foo', 'foo is very important to be in this string')`

The easiest way to debug a failing test is to run it in a browser and use
DevTools.
To facilitate that, run:

    make browser_test

and navigate to <http://localhost:9876/debug.html>. If you are running on a
CloudTop then you should repeat the ssh port forwarding steps mentioned in the
`make serve` instructions.

A suggested workflow to deal with the code is to add a few breakpoints, then
use the log messages to access the lines inside the source.
Then, add breakpoints and go from there.

When debugging certain tests, it may be useful to prefix the `it` or `describe`
statement with a letter 'f' (for force). Then, only tests with the 'f' prefix
will run. Conversely, tests can be disabled with an 'x' prefix.

If you are running on a local environment and get a disconnection error in the form: `Disconnectedreconnect failed before timeout of X ms`, try using the following custom launcher (which can be added to karma.conf.js):

    customLaunchers: {
        'ChromeHeadless': {
            base: 'Chrome',
            flags: [
                '--headless',
                '--disable-gpu',
                '--no-sandbox',
                '--disable-web-security',
                // Without a debugging port, Chrome exits immediately.
                '--remote-debugging-port=9222'
            ],
            debug: true
        }
    },

## Generating the docs

We use [JSDoc](http://usejsdoc.org) to document the modules. While the
documentation is readable inline, it can be easier to browse in a web browser.

To generate the HTML docs, run

    make docs

which will open docs/index.html after build.
