# TTQ

Stands for Transactional Task enQueueing Library.

## Code structure

  * `ttq` is the top level package contains only general publicly expose types.
    Must not depend on specific datastore implementation.
  * `  internal/` contains actual implementation against abstract Database
    interface.
  * `  ttqdatastore/` Datastore implementation.
  * `  ttqspanner/`   Spanner implementation.

The intent is avoid linking against datastore for apps which only use Spanner
and vise versa.
