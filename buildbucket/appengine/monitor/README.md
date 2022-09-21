# Buildbucket Monitor

Buildbucket Monitor is a micro-service that periodically computes and reports
aggregate views of Build stats. For example:

- The maximum pending time in a given Builder
- Number of Builds per each status in a given Builder

# How does this work?

A cron request is triggered periodically. The cron handler computes
and reports aggregate views of Build events, using a manual flush().

## Why manual flush()?

It's to avoid unnecessary resets on the in-memory metric storage.
Once metric data is set, tsmon reports the last-seen data repeatedly
on every single flush.

If multiple Appengine instances handles a task of computing aggregated
views, the following problem would occur:

(hh:mm)
1. 00:00, Ins-1 computed and reported the views for Builder A
2. 00:01,
    - Ins-1 reported the views for Builder A, computed at the time of 00:00.
    - Ins-2 computed and reported the views for Builder A.
3. 00:02,
    - Ins-1 reported the views for Builder A, computed at the time of 00:00
    - Ins-2 reported the views for Builder A, computed at the time of 00:01

In the above scenario, there is no way to determine which stream tells
the most fresh data. To avoid this issue, this module manually flushes
the data with proper cleanup beforehand.
