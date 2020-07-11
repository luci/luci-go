# TTQ

Stands for Transactional Task enQueueing Library.

## Code structure

  * `ttq` is the top level package contains only general publicly expose types.
    Must not depend on specific datastore implementation.
  * `  internal/` contains actual implementation.
  * `  internal/databases/` contains specialization for different databases.
  * `  internal/sweepdrivers/` contains specialization for different sweep
    drivers.
  * TODO(tandrii): document high level usable packages.

The intent is avoid linking against datastore for apps which only use Spanner
and vise versa.
