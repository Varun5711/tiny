# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the Tiny URL Shortener project.

## What is an ADR?

An ADR is a document that captures an important architectural decision along with its context and consequences.

## ADR Index

| ID | Title | Status | Date |
|----|-------|--------|------|
| [001](./001-snowflake-id-generation.md) | Snowflake ID Generation | Accepted | 2024-12 |
| [002](./002-oltp-olap-separation.md) | OLTP/OLAP Database Separation | Accepted | 2024-12 |
| [003](./003-redis-streams-for-events.md) | Redis Streams for Event Processing | Accepted | 2024-12 |
| [004](./004-grpc-for-internal-communication.md) | gRPC for Internal Communication | Accepted | 2024-12 |
| [005](./005-multi-tier-caching.md) | Multi-Tier Caching Strategy | Accepted | 2024-12 |
| [006](./006-postgresql-read-replicas.md) | PostgreSQL Read Replicas | Accepted | 2024-12 |
| [007](./007-jwt-authentication.md) | JWT for Authentication | Accepted | 2024-12 |

## ADR Status

- **Proposed** - Under discussion
- **Accepted** - Approved and implemented
- **Deprecated** - No longer valid
- **Superseded** - Replaced by another ADR

## Template

```markdown
# ADR-XXX: Title

## Status
Accepted | Deprecated | Superseded

## Context
What is the issue that we're seeing that is motivating this decision?

## Decision
What is the change that we're proposing and/or doing?

## Consequences
What becomes easier or more difficult because of this change?
```
