# Architecture — package boundaries

`DOMAIN_WORLD_SPLIT_PATCH` (slice 3)

## Layout

```
cmd/                       entrypoints only (realmd, worldd, wowctl)
internal/app/              composition roots (realmd, worldd)
internal/config/           flag/file/env parsing + validation
internal/domain/           pure models + invariants (no SQL, no packets)
  account/                 account model, access decision, login session snapshot
  realm/                   realm model
  character/               character models, creation policy, player snapshot
internal/application/      use-cases / services (consumer-side interfaces only)
  character/               character creation service
  character/tbc/           build 8606 creation profile
  world/                   phase-10 ports: World, Runner, Intent, Event, Outbox
internal/protocol/         codec, framing, session, handlers (orchestration only)
  auth/                    legacy login envelope
  auth/tbc/                build 8606 login: SRP, realms, reconnect
  wire/                    version-independent ByteBuffer reader/writer
  world/                   version-independent packet envelope
  world/tbc/               build 8606 session FSM, router, route handlers
  world/tbc/frame/         packet framing + single-owner stream Reader/Writer
  world/tbc/crypt/         header cipher (AuthCrypt port)
  world/tbc/codec/         pure encode/decode, no I/O
  world/tbc/opcodes/       generated opcode metadata
  world/tbc/updatefields/  generated object update fields
internal/world/            phase-10 simulation loop (contract only for now; doc.go)
internal/storage/          DB adapters implementing application ports
  realmd/                  accounts, sessions, realms
  characters/              character persistence
    mangosone8606/         build-8606 item_instance field adapter
  world/                   world DB reads for static data
  sqlutil/                 shared MySQL open/check helpers
internal/gamedata/         immutable static data (DBC + templates); tbc/ loader
internal/transport/        accept loops / connection limits
internal/admin/            wowctl commands
```

## Import rules

Allowed:

| From             | May import                                        |
|------------------|---------------------------------------------------|
| cmd              | internal/app, internal/config, internal/admin     |
| app              | everything (composition root)                     |
| application      | domain, gamedata (read-only)                      |
| domain           | gamedata (value types only), stdlib               |
| protocol         | domain, application (interfaces), wire            |
| world (phase 10) | gamedata (read-only); application/world ports are |
|                  | satisfied structurally without importing them     |
| storage          | domain, sqlutil                                   |
| gamedata         | stdlib, dbc                                       |

Forbidden:

- `domain` must not import `protocol`, `storage`, `application`, `world`, `app`
- `application` must not import `protocol`, `storage`, `world`, `app`
- `world` must not import `protocol`, `storage`, `application`, `app`
- `protocol` must not import `storage` packages, `world` or `app`
- `storage` must not import `protocol`

Handlers in `protocol/...` only: decode → call application/domain ports → encode.

## Account & realm flow

- Account model, access decisions, login session snapshot: `internal/domain/account`
- Realm model: `internal/domain/realm`
- Persistence: `internal/storage/realmd`
- SRP6: `internal/auth/srp` (version-independent, vector-tested)

## Character flow

- Types / errors / policy facts: `internal/domain/character`
- `Service.Create`: `internal/application/character`
- TBC profile (build 8606): `internal/application/character/tbc`
- Persistence: `internal/storage/characters` (+ `mangosone8606` item adapter)

## World flow (phase 10 sketch)

- Ports and contracts: `internal/application/world` (World, Runner, Intent, Event, Outbox)
- Loop and map state: `internal/world` — one owner goroutine, no shared locks;
  see the package documentation there.
- Protocol routes decode packets into `Intent` values and `Submit` them; a
  protocol-side pump encodes `Event` values into build-specific packets. Game
  rules never live in protocol packages.
