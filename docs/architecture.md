# Architecture — package boundaries

`DOMAIN_USECASES_PATCH` (slice 4)

## Layout

```
cmd/                       entrypoints only (realmd, worldd, wowctl)
internal/app/              composition roots (realmd, worldd)
internal/config/           flag/file/env parsing + validation
internal/domain/           pure models + invariants (no SQL, no packets)
  account/                 account model, access decision, login session snapshot
  realm/                   realm model
  character/               character models, creation policy, player snapshot
  world/                   phase-10 anchor: simulation-owned rules (doc only)
internal/application/      use-cases / services (consumer-side interfaces only)
  auth/                    phase-10 anchor: login use cases (doc only)
  character/               temporary compat shim (remove in slice 5)
  character/create/        character creation use case
  character/create/tbc/    build 8606 creation profile
  character/list/          character enumeration use case
  world/                   phase-10 ports: World, Runner, Intent, Event, Outbox
  worldlogin/              world-entry use cases (AcquirePlayer/ReleasePlayer)
  chat/                    phase-10 anchor: chat use cases (doc only)
  movement/                phase-10 anchor: movement use cases (doc only)
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
  world/tbc/routes/        phase-10 route home (doc only)
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

## Deliberate deviations from the target sketch

- `internal/auth/srp` stays a version-independent library under internal/:
  storage/realmd consumes it too, and storage must never import protocol.
- `internal/dbc` stays: DBC file-format parser consumed by gamedata and the
  creation profile.
- `internal/testclient`, `protocol/wire`, `protocol/client`,
  `world/tbc/updatefields`, `storage/sqlutil`: support packages outside the
  sketched tree; each has a single clear owner layer.

## Use-case ownership (slice 4)

- Character creation: protocol route → `application/character/create.Service`
  (build profile in `create/tbc`, policy facts in `domain/character`,
  persistence in `storage/characters`).
- Character enumeration: protocol route → `application/character/list.Service`
  (account guard + TBC capacity bound owned by the use case).
- World entry/exit: protocol route → `application/worldlogin.Service`
  (acquire/release decisions owned by the use case; the session FSM stays in
  protocol).
- Login/auth: unchanged; ban/lock/IP decisions stay in `domain/account`.
- Phase 10: chat and movement use cases will live in `application/chat` and
  `application/movement`, their rules in `domain/world`, their packet routes
  in `protocol/world/tbc/routes`; Intents/Events flow through the
  `application/world` ports.

## Cleanup backlog (slice 5)

- Remove `application/character/compat.go` once callers import
  `application/character/create` directly.
- Remove `protocol/world/tbc/split_compat.go` once the tbc session package
  imports `frame`/`crypt` subpackages directly.

## Account & realm flow

- Account model, access decisions, login session snapshot: `internal/domain/account`
- Realm model: `internal/domain/realm`
- Persistence: `internal/storage/realmd`
- SRP6: `internal/auth/srp` (version-independent, vector-tested)

## Character flow

- Types / errors / policy facts: `internal/domain/character`
- Creation use case: `internal/application/character/create` (+ `create/tbc` profile)
- Enumeration use case: `internal/application/character/list`
- Persistence: `internal/storage/characters` (+ `mangosone8606` item adapter)

## World flow (phase 10 sketch)

- Ports and contracts: `internal/application/world` (World, Runner, Intent, Event, Outbox)
- World-entry use cases: `internal/application/worldlogin`
- Loop and map state: `internal/world` — one owner goroutine, no shared locks;
  see the package documentation there.
- Protocol routes decode packets into `Intent` values and `Submit` them; a
  protocol-side pump encodes `Event` values into build-specific packets. Game
  rules never live in protocol packages.
