# Redis Documentation

This directory collects Redis-specific technical documentation for the current
project state. It intentionally keeps only live technical docs; historical
operational runbooks and execution records are omitted because MPP has not had a
real production Redis change window.

## Documents

| Document | Purpose |
| --- | --- |
| [Redis dependency map](./redis-dependency-map.md) | Maps each Redis-dependent service to its runtime role, failure behavior, and follow-up direction. |
| [Redis Cluster compatibility audit](./redis-cluster-compatibility-audit.md) | Lists current Cluster blockers in clients, key shapes, commands, scripts, queues, pub/sub, and operational tooling. |
| [Redis key hash-tag convention](./redis-key-hash-tag-convention.md) | Defines the hash-tag rules used to keep related Redis keys in one slot for Cluster-safe grouped operations. |

## Related Plan

- [Redis availability notes](../plan/redis-availability-plan.md) stays in
  `doc/plan/` because it is the entry point for the broader reliability work.
