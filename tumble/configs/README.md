## Tumble Configuration Files

This directory contains configuration files for `luci_deploy`.

### Task Queue

The task queue configuration files (`tq_shards_*`) configure the Tumble task
queue. As Tumble accumulates mutations, it adds tasks to the task queue. The
queue throughput is a function of Tumble's active configuration, namely the
number of shards, the number of namespaces, and TemporalRoundFactor that is
employed.
