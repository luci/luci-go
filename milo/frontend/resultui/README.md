# ResultUI
ResultUI is an attempt to rethink the Milo build page, with a focus on test results served from the new ResultDB backend.
It aims to provide Chrome developers a convenient way to view the output of failing tests.

## Main Features
* Test results page  
  Test results page gives a cleaner view of all the tests run by a task, including why they failed, the test configurations, and any artifacts produced.

## Contribution
See the [contribution guide](docs/contribution.md).

## Q&A
### Why is part of the test ID greyed out?
If a test shares a common prefix with the previous test, the common prefix is greyed out. This makes it easier to scan through a long list of tests.
### Why there were no new tests after I clicked load more?
Due to a technical limitation, newly loaded test results are inserted into the list instead of appended to the end of the list. With the default filter setting, all of the tests should be loaded in the first page most of the time. So this will be less of an issue.
### How do I provide feedback?
There's a feedback button in the top right corner of the page. It will take you to Monorail with a pre-populated template. Feedback is always welcomed :)
