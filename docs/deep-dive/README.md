# Tiny URL Shortener: Deep-Dive Documentary Series

> **A comprehensive, educational journey through system design decisions**

This series explains **WHY** every architectural decision was made in the Tiny URL Shortener system - not just what was built, but the reasoning, trade-offs, and alternatives considered for each choice.

## Overview

This is not just reference documentation. It's a documentary-style exploration of building a production-ready URL shortener, answering questions like:
- Why Redis Streams instead of Kafka?
- Why Snowflake IDs and Base62 encoding?
- Why multi-tier caching (L1 + L2)?
- Why PostgreSQL + ClickHouse instead of just one database?
- Why 8 microservices instead of a monolith?
- Why gRPC internally but REST externally?

Each document combines:
- **Line-by-line code analysis** of critical sections
- **Architecture diagrams** (Mermaid) showing how components connect
- **Comparison tables** evaluating alternatives
- **"Why not X?"** sections explaining rejected options
- **Real-world examples** using consistent scenarios
- **Trade-off discussions** - being honest about downsides

## Document Series

### 📚 Status: Complete Series (35,700 words)

1. ✅ [The Big Picture](./01-big-picture.md) - **4,100 words**
   - System overview, microservices rationale, data flows, technology choices

2. ✅ [Database Architecture Deep Dive](./02-database-architecture.md) - **5,100 words**
   - PostgreSQL replication, ClickHouse OLAP, Redis patterns, read/write splitting

3. ✅ [Messaging & Queuing: Why Redis Streams?](./03-messaging-queuing.md) - **3,700 words**
   - Redis Streams vs Kafka comparison, consumer groups, at-least-once delivery

4. ✅ [Multi-Tier Caching Strategy](./04-caching-strategy.md) - **3,100 words**
   - L1 LRU + L2 Redis architecture, cache invalidation, performance analysis

5. ✅ [Short Code Generation (Snowflake + Base62)](./05-short-code-generation.md) - **3,500 words**
   - Snowflake ID algorithm, Base62 encoding, collision-free guarantees

6. ✅ [gRPC vs REST: Internal Communication](./06-grpc-internal-communication.md) - **2,100 words**
   - Protocol comparison, API Gateway pattern, proto definitions

7. ✅ [Authentication & JWT Tokens](./07-authentication-jwt.md) - **2,200 words**
   - JWT structure, stateless auth, bcrypt hashing, security considerations

8. ✅ [Rate Limiting with Sliding Window](./08-rate-limiting.md) - **2,300 words**
   - Redis sorted sets, sliding window algorithm, fair rate limiting

9. ✅ [Workers & Background Processing](./09-workers.md) - **2,600 words**
   - Analytics Worker, Pipeline Worker, Cleanup Worker, fault isolation

10. ✅ [Code Walkthrough: URL Creation End-to-End](./10-code-walkthrough-create-url.md) - **2,300 words**
    - Complete flow from HTTP POST to database insert with timing breakdown

11. ✅ [Code Walkthrough: Redirect & Click Tracking](./11-code-walkthrough-redirect.md) - **2,200 words**
    - Multi-tier cache lookup, async event publishing, background enrichment

12. ✅ [Scaling Strategy](./12-scaling-strategy.md) - **2,500 words**
    - Bottleneck analysis, horizontal scaling, database sharding, cost projections

## Suggested Reading Paths

### 🎓 For Beginners
Start here if you're new to system design or microservices:
1. [The Big Picture](./01-big-picture.md) - Understand the overall system
2. [Messaging & Queuing](./03-messaging-queuing.md) - Learn about event-driven architecture
3. [Multi-Tier Caching](./04-caching-strategy.md) - Understand performance optimization
4. [URL Creation Walkthrough](./10-code-walkthrough-create-url.md) - See it all in action
5. [Redirect Walkthrough](./11-code-walkthrough-redirect.md) - Complete the picture

### 💼 For Intermediate Developers
Deep dive into specific components:
1. [Database Architecture](./02-database-architecture.md) - PostgreSQL, ClickHouse, Redis
2. [Short Code Generation](./05-short-code-generation.md) - Snowflake IDs & Base62
3. [gRPC Communication](./06-grpc-internal-communication.md) - Service-to-service patterns
4. [Authentication & JWT](./07-authentication-jwt.md) - Security implementation
5. [Rate Limiting](./08-rate-limiting.md) - Protecting your API
6. [Workers & Background Jobs](./09-workers.md) - Async processing

### 🚀 For Advanced Engineers
System design and scalability:
1. [The Big Picture](./01-big-picture.md) - Microservices vs monolith rationale
2. [Database Architecture](./02-database-architecture.md) - Read/write splitting, sharding strategies
3. [Messaging & Queuing](./03-messaging-queuing.md) - When to use Kafka vs Redis Streams
4. [Scaling Strategy](./12-scaling-strategy.md) - Bottlenecks and solutions

### 🗄️ Database-Focused Path
For those interested in data architecture:
1. [Database Architecture](./02-database-architecture.md) - Three data stores explained
2. [Messaging & Queuing](./03-messaging-queuing.md) - Event sourcing patterns
3. [Workers](./09-workers.md) - Data pipeline and ETL

### 🔒 Security-Focused Path
For security-minded developers:
1. [Authentication & JWT](./07-authentication-jwt.md) - Token-based auth
2. [Rate Limiting](./08-rate-limiting.md) - Protecting against abuse

## Document Structure

Each document follows a consistent pattern:
1. **Overview** - What problem are we solving?
2. **Implementation Walkthrough** - How it's built (with code)
3. **Design Decisions** - Why this approach? What alternatives were considered?
4. **Real-World Implications** - Performance, scaling, and trade-offs
5. **Summary & Connections** - Key takeaways and links to related docs

## System Architecture at a Glance

```
                          ┌─────────────────┐
                          │  API Gateway    │ ◄─── External REST API
                          │  (port 8080)    │
                          └────────┬────────┘
                                   │ gRPC
                ┌──────────────────┼──────────────────┐
                │                  │                  │
        ┌───────▼────────┐ ┌──────▼──────┐  ┌───────▼────────┐
        │  URL Service   │ │ User Service │  │Redirect Service│
        │ (port 50051)   │ │ (port 50052) │  │  (port 8081)   │
        └───────┬────────┘ └──────┬───────┘  └───────┬────────┘
                │                 │                   │
                └─────────┬───────┘                   │
                          │                           │
                    ┌─────▼─────┐            ┌────────▼────────┐
                    │PostgreSQL │            │  Redis Streams  │
                    │Primary +  │            │ (click events)  │
                    │3 Replicas │            └────────┬────────┘
                    └───────────┘                     │
                                          ┌───────────┴───────────┐
                                          │                       │
                                  ┌───────▼────────┐  ┌──────────▼─────────┐
                                  │Analytics Worker│  │  Pipeline Worker   │
                                  │(PG updates)    │  │(GeoIP + ClickHouse)│
                                  └────────────────┘  └────────────────────┘
```

**8 Microservices:**
- API Gateway - REST to gRPC translation
- URL Service - Core shortening logic
- User Service - Authentication
- Redirect Service - High-performance redirects
- Analytics Worker - Click count aggregation
- Pipeline Worker - Event enrichment & analytics
- Cleanup Worker - Expired URL deletion
- TUI - Terminal client

**3 Data Stores:**
- PostgreSQL (1 primary + 3 replicas) - OLTP
- ClickHouse - OLAP analytics
- Redis - Cache + Streams + Rate limiting

## Key Technologies

- **Language**: Go 1.25.3
- **Communication**: gRPC (internal), REST (external)
- **Databases**: PostgreSQL (pgx/v5), ClickHouse, Redis
- **Patterns**: Event-driven (Redis Streams), multi-tier caching, read/write splitting
- **ID Generation**: Snowflake algorithm → Base62 encoding

## How to Use This Series

**Read Sequentially**: Documents build on each other, with cross-references throughout.

**Jump to Topics**: Each document stands alone if you need specific information.

**Code References**: All code snippets reference actual files in the codebase. Look for paths like:
```
internal/idgen/snowflake.go:45
```

**Ask Questions**: If something is unclear, the goal of this series is education. Consider opening an issue or PR to improve explanations.

## Contributing

Found an error? Have a suggestion? Want to add a diagram?
- Open an issue describing the improvement
- Submit a PR with your changes
- Maintain the documentary style and educational tone

## About This Series

**Goal**: Understand every architectural decision deeply - the "why" behind the "what".

**Style**: Conversational, honest about trade-offs, and educational.

**Audience**: Developers learning system design, engineers evaluating similar architectures, or anyone curious about building production services.

---

**Documentation reflects codebase at commit**: `0ab60a8` (feat/refactor branch)

**Completed**: 2025-12-13

**Total Word Count**: 35,700 words across 12 documents

**Estimated Reading Time**: ~2.5-3 hours for complete series (~15 minutes per document)
