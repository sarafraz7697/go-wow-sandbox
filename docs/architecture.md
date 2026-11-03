# Architecture — package boundaries

`PACKAGE_SKELETON_PATCH` (slice 2)

## Layout

```
cmd/                    entrypoints only
internal/app/           composition roots (realmd, worldd)
internal/domain/        pure models + invariants (no SQL, no packets)
internal/application/   use-cases / services (interfaces in, no concrete SQL drivers)
internal/protocol/      codec, framing, session, handlers (orchestration only)
internal/storage/       DB adapters implementing application ports
internal/gamedata/      immutable static data (DBC + templates)
internal/transport/     accept loops / connection limits
```

## Import rules

Allowed:

| From            | May import                                      |
|-----------------|--------------------------------------------------|
| cmd             | internal/app, internal/config, internal/admin    |
| app             | everything (composition root)                    |
| application     | domain, account (ports), gamedata (read-only)    |
| domain          | gamedata (value types only), stdlib              |
| protocol        | domain, application (interfaces), account, wire  |
| storage         | domain, account, sqlutil                         |
| gamedata        | stdlib, dbc                                      |

Forbidden:

- `domain` must not import `protocol`, `storage`, `application`, `app`
- `protocol` must not import `storage` concrete drivers (only interfaces defined in protocol)
- `application` must not import `protocol` or concrete `storage` packages
- `storage` must not import `protocol`

Handlers in `protocol/...` only: decode → call application/domain ports → encode.

## Character flow

- Types / errors / policy facts: `internal/domain/character`
- `Service.Create`: `internal/application/character`
- TBC profile (build 8606): `internal/application/character/tbc`
- Persistence: `internal/storage/characters`
