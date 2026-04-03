# HyperCache Documentation

## Architecture
Deep dives into core algorithms and system design.

- [Code Structure](architecture/code-structure.md) — Package layout and responsibilities
- [Consistent Hashing](architecture/consistent-hashing-deep-dive.md) — Hash ring with virtual nodes
- [Raft Consensus](architecture/raft-consensus-deep-dive.md) — Distributed consensus protocol
- [Cuckoo Filter Internals](architecture/cuckoo-filter-internals.md) — Probabilistic data structure design
- [RESP Protocol](architecture/resp-protocol-specification.md) — Redis Serialization Protocol implementation
- [Binary Protocol](architecture/binary-protocol-specification.md) — Internal cluster communication protocol
- [Distributed Persistence](architecture/distributed-persistence-plan.md) — AOF + Snapshot coordination

## Guides
Step-by-step instructions for setup, deployment, and operations.

- [Development Setup](guides/development-setup.md) — Prerequisites and local dev environment
- [Implementation Guide](guides/implementation-guide.md) — Core components walkthrough
- [Docker Guide](guides/docker-guide.md) — Build, deploy, and test with Docker/Compose
- [Multi-VM Deployment](guides/multi-vm-deployment-guide.md) — Cloud and bare-metal cluster deployment
- [Observability](guides/observability.md) — Logging, Elasticsearch, Grafana, Filebeat
- [Redis CLI Testing](guides/redis-cli-testing-guide.md) — Manual testing with redis-cli
- [Contribution Guidelines](guides/contribution-guidelines.md) — How to contribute

## Reference
Quick-reference materials and operational data.

- [Performance Benchmarks](reference/performance-benchmarks.md) — Latency and throughput numbers
- [Persistence File Locations](reference/persistence-file-locations.md) — AOF/snapshot file paths per node
- [Known Issues](reference/KNOWN_ISSUES.md) — Tracked issues and resolutions
- [Scripts Reference](reference/SCRIPTS_README.md) — Operational scripts documentation
