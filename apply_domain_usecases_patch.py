#!/usr/bin/env python3
"""
apply_domain_usecases_patch.py

Slice 4 — application use-case layer + phase-10 domain anchors.

Completes the domain-based tree the project agreed on. Business logic moves
out of protocol reach into application use cases; the protocol layer keeps
only decode -> use case -> encode. No behavior changes.

1. Splits the character application package into explicit use cases:

       internal/application/character/create/       (package create)
           service.go   — moved from internal/application/character
                          (CreationProfile, CreationStore, Service, NewService)
       internal/application/character/create/tbc/   (package tbc)
           creation.go  — moved from internal/application/character/tbc
                          (build-8606 creation profile, whole directory)
       internal/application/character/list/         (package list)
           service.go   — NEW enumeration use case: account guard + TBC
                          MaxCharacters bound owned by the use case, storage
                          behind a consumer-side Reader port

   A temporary shim keeps the old package name compiling:

       internal/application/character/compat.go      (package character)
                          — aliases Service/NewService/CreationProfile/
                          CreationStore to the create package. Remove in the
                          next cleanup slice once callers import create
                          directly. White-box tests constructing Service
                          literals with unexported fields must move to
                          package create.

2. Adds the world-entry use case (phase 9 seam, phase 10 growth point):

       internal/application/worldlogin/service.go    (package worldlogin)
                          — Players port (consumer-side) + Service with
                          AcquirePlayer/ReleasePlayer. Method names
                          intentionally match the protocol-side PlayerStore
                          port, so the service satisfies it structurally and
                          protocol/world/tbc/player.go stays untouched.

3. Adds the phase-10 anchors as documentation-only packages:

       internal/application/auth/doc.go              login use cases (future)
       internal/application/chat/doc.go              chat use cases (phase 10)
       internal/application/movement/doc.go          movement use cases (phase 10)
       internal/domain/world/doc.go                  simulation-owned rules
       internal/protocol/world/tbc/routes/doc.go     phase-10 route home

4. Rewires the composition root (internal/app/worldd.go):

       h.Creator    = create.NewService(profile, creator)
       h.Characters = list.NewService(chars)      (was storage store directly)
       h.Players    = worldlogin.NewService(chars) (was storage store directly)

5. Rewrites the moved profile import path across the tree (including tests):

       internal/application/character/tbc  ->  .../character/create/tbc

6. Refreshes docs/architecture.md (layout, deviations from the target sketch,
   use-case ownership map).

Safety:
  * all-or-nothing staging (nothing written until every step succeeds)
  * backup <file>.skel.bak before first overwrite/delete (existing backups
    from earlier slices are never overwritten)
  * idempotent when the new tree already exists and the old tree is gone
  * preserves CRLF line endings and tab indentation of the existing files
  * refuses to touch files that drifted from the expected anchors

Usage:
    python apply_domain_usecases_patch.py /path/to/wowserver
"""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

MARK = "DOMAIN_USECASES_PATCH"
BAK_SUFFIX = ".skel.bak"
SKIP_DIRS = {".git", "bin", "vendor", "node_modules", "__pycache__", "dbc"}

OLD_APPCHAR = "internal/application/character"
NEW_CREATE = "internal/application/character/create"
OLD_TBC_PROFILE = "internal/application/character/tbc"
NEW_TBC_PROFILE = "internal/application/character/create/tbc"

OLD_PROFILE_IMPORT = '"wowserver/internal/application/character/tbc"'
NEW_PROFILE_IMPORT = '"wowserver/internal/application/character/create/tbc"'


class PatchError(Exception):
    pass


def read_text(path: Path):
    raw = path.read_bytes()
    text = raw.decode("utf-8", errors="surrogateescape")
    crlf = "\r\n" in text
    return text.replace("\r\n", "\n"), crlf


def write_text(path: Path, text: str, crlf: bool):
    path.parent.mkdir(parents=True, exist_ok=True)
    if crlf:
        text = text.replace("\n", "\r\n")
    path.write_bytes(text.encode("utf-8", errors="surrogateescape"))


def tabs(text: str) -> str:
    """Convert leading 4-space groups to tabs (repo uses tab indentation)."""
    import re

    text = re.sub(r"^(?:    )+", lambda m: "\t" * (len(m.group(0)) // 4), text, flags=re.M)
    return text.rstrip("\n") + "\n"


def collect_go_files(root: Path):
    out = []
    for p in root.rglob("*.go"):
        if any(part in SKIP_DIRS for part in p.parts):
            continue
        out.append(p)
    return out


def backup_once(path: Path):
    if not path.exists():
        return
    bak = path.with_name(path.name + BAK_SUFFIX)
    if not bak.exists():
        shutil.copy2(str(path), str(bak))


def count_occurrences(text: str, needle: str) -> int:
    return text.count(needle)


def replace_once(text: str, old: str, new: str, what: str) -> str:
    n = count_occurrences(text, old)
    if n != 1:
        raise PatchError(
            "expected exactly one occurrence of %r, found %d" % (what, n)
        )
    return text.replace(old, new, 1)


def remove_empty_dir(path: Path):
    if path.is_dir() and not any(path.iterdir()):
        path.rmdir()
        print("removed  %s/" % path)


# --------------------------------------------------------------------------
# Embedded new files. Go contents use 4-space indentation here and are
# converted to tabs by tabs() before writing.
# --------------------------------------------------------------------------

COMPAT_GO = tabs('''package character

// DOMAIN_USECASES_PATCH: temporary re-export shim. The use case moved to
// internal/application/character/create; this alias file keeps existing
// callers and package tests compiling unchanged. Remove it in the next
// cleanup slice once call sites and tests import the create package
// directly. White-box tests that construct Service literals with unexported
// fields must move to package create.

import create "wowserver/internal/application/character/create"

type CreationProfile = create.CreationProfile
type CreationStore = create.CreationStore
type Service = create.Service

var NewService = create.NewService
''')

LIST_SERVICE_GO = tabs('''// Package list is the character-screen enumeration use case.
//
// DOMAIN_USECASES_PATCH: the account guard and the packet-capacity bound are
// use-case rules and live here; storage stays a replaceable port. Protocol
// handlers call this service and only decide how to serialize the result.
package list

import (
    "context"
    "errors"

    character "wowserver/internal/domain/character"
)

// MaxCharacters matches the TBC SMSG_CHAR_ENUM count byte. The storage port
// enforces the same bound against the database; this is the use-case side.
const MaxCharacters = 255

// Reader is the persistence port of this use case, defined consumer-side.
// Implemented by storage/characters.Store.
type Reader interface {
    List(context.Context, uint64) ([]character.Summary, error)
}

// Service loads the verified account's characters for the character screen.
type Service struct {
    reader Reader
}

func NewService(reader Reader) *Service {
    return &Service{reader: reader}
}

func (s *Service) List(ctx context.Context, accountID uint64) ([]character.Summary, error) {
    if s.reader == nil {
        return nil, errors.New("character list store not configured")
    }
    if accountID == 0 {
        return nil, errors.New("invalid character account")
    }
    rows, err := s.reader.List(ctx, accountID)
    if err != nil {
        return nil, err
    }
    if len(rows) > MaxCharacters {
        return nil, errors.New("character count exceeds TBC packet capacity")
    }
    return rows, nil
}
''')

WORLDLOGIN_SERVICE_GO = tabs('''// Package worldlogin owns the world-entry use cases (phase 9 seam, phase 10
// growth point).
//
// DOMAIN_USECASES_PATCH: protocol routes keep the session FSM and packet
// sequencing; the decisions about which player snapshot may attach to a
// verified session live here. When the phase-10 loop lands, relogin and
// admission rules join this package instead of protocol handlers.
package worldlogin

import (
    "context"
    "errors"

    character "wowserver/internal/domain/character"
)

// Players is the persistence port of this use case, defined consumer-side.
// Implemented by storage/characters.Store.
type Players interface {
    AcquirePlayer(context.Context, uint64, uint32) (*character.Player, error)
    ReleasePlayer(context.Context, uint64, *character.Player, uint32) error
}

// Service attaches and detaches loaded player snapshots to verified sessions.
// The method names intentionally match the protocol-side PlayerStore port so
// the service satisfies it structurally.
type Service struct {
    players Players
}

func NewService(players Players) *Service {
    return &Service{players: players}
}

// AcquirePlayer loads and reserves the account's player for world entry.
// A zero account or GUID is never valid; ownership, online-state and
// supported-shape rules are enforced by the port within its transaction.
func (s *Service) AcquirePlayer(ctx context.Context, accountID uint64, guid uint32) (*character.Player, error) {
    if s.players == nil {
        return nil, errors.New("world login store not configured")
    }
    if accountID == 0 || guid == 0 {
        return nil, character.ErrPlayerUnavailable
    }
    return s.players.AcquirePlayer(ctx, accountID, guid)
}

// ReleasePlayer saves and releases the snapshot at logout or teardown.
func (s *Service) ReleasePlayer(ctx context.Context, accountID uint64, p *character.Player, seconds uint32) error {
    if s.players == nil {
        return errors.New("world login store not configured")
    }
    if p == nil {
        return nil
    }
    return s.players.ReleasePlayer(ctx, accountID, p, seconds)
}
''')

APP_AUTH_DOC_GO = tabs('''// Package auth is the reserved home for the login use cases.
//
// DOMAIN_USECASES_PATCH: contract only. Today the ban/lock/IP decisions run
// in internal/domain/account and the SRP handshake stays in protocol/auth/tbc.
// When login grows orchestration of its own (failed-login throttling policy,
// session publishing rules, reconnect admission), it moves here as use cases
// that protocol/auth/tbc calls. This package must not import protocol,
// storage, world or app.
package auth
''')

APP_CHAT_DOC_GO = tabs('''// Package chat is the reserved home for the phase-10 chat use cases
// (say/yell/party lines).
//
// DOMAIN_USECASES_PATCH: contract only. Channel, language and spam rules must
// live here (or in domain/world), never in protocol route handlers. Routes
// decode packets into application/world Intents (IntentChat) and encode
// Events; this package must not import protocol, storage or app.
package chat
''')

APP_MOVEMENT_DOC_GO = tabs('''// Package movement is the reserved home for the phase-10 movement use cases.
//
// DOMAIN_USECASES_PATCH: contract only. Report validation policy and the
// anti-cheat rules that decide which client movement may mutate world state
// must live here (or in domain/world), never in protocol route handlers.
// Routes decode MSG_MOVE_* packets into application/world Intents
// (IntentMove); this package must not import protocol, storage or app.
package movement
''')

DOMAIN_WORLD_DOC_GO = tabs('''// Package world will host the phase-10 domain model owned by the simulation:
// world objects and their invariants, movement and chat rules as plain
// values, and the player-in-world state transitions the loop applies.
//
// DOMAIN_USECASES_PATCH: contract only. Types here are pure values with
// invariants — no I/O, no packets, no SQL. One goroutine owns all mutable
// state at runtime (see internal/world/doc.go); this package must not import
// protocol, storage, application, world or app.
package world
''')

ROUTES_DOC_GO = tabs('''// Package routes is the reserved home for phase-10 packet routes.
//
// DOMAIN_USECASES_PATCH: contract only. A route here may only decode a
// packet, call an application use case or submit an application/world Intent,
// and encode the reply (decode -> use case -> encode). It must not import
// storage, internal/world or app, and must not reach into session FSM
// internals: state-dependent handlers stay registered by the tbc session
// package until the simulation loop takes over.
package routes
''')

ARCHITECTURE_MD = '''# Architecture — package boundaries

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
'''


# --------------------------------------------------------------------------
# Source discovery and adaptation
# --------------------------------------------------------------------------

def adapt_create_service(text: str) -> str:
    prefix = "package character\n"
    if not text.startswith(prefix):
        raise PatchError("application/character/service.go does not start with 'package character'")
    text = (
        "package create\n"
        "\n"
        "// DOMAIN_USECASES_PATCH: moved from internal/application/character. The\n"
        "// old package name remains only as a temporary shim (compat.go there).\n"
        + text[len(prefix):]
    )
    old_mark = "// PACKAGE_SKELETON_PATCH\n"
    if old_mark in text:
        text = text.replace(
            old_mark,
            "// Application service: coordinates profile validation and persistence.\n",
            1,
        )
    return text


def adapt_moved_profile(text: str) -> str:
    text = text.replace(OLD_PROFILE_IMPORT, NEW_PROFILE_IMPORT)
    old_mark = "// PACKAGE_SKELETON_PATCH: profile lives under application/character/tbc\n"
    if old_mark in text:
        text = text.replace(
            old_mark,
            "// DOMAIN_USECASES_PATCH: moved from internal/application/character/tbc\n"
            "// to internal/application/character/create/tbc.\n",
            1,
        )
    return text


def patch_worldd(text: str) -> str:
    new_markers = (
        'charcreate "wowserver/internal/application/character/create"',
        "charlist.NewService(chars)",
        "charcreate.NewService(profile, creator)",
        "worldlogin.NewService(chars)",
    )
    if all(m in text for m in new_markers) and 'charapp "' not in text:
        return text  # already wired by a previous run; nothing to do
    old_imports = (
        '\tcharapp "wowserver/internal/application/character"\n'
        '\tcreationTBC "wowserver/internal/application/character/tbc"\n'
    )
    new_imports = (
        '\tcharcreate "wowserver/internal/application/character/create"\n'
        '\tcreationTBC "wowserver/internal/application/character/create/tbc"\n'
        '\tcharlist "wowserver/internal/application/character/list"\n'
        '\tworldlogin "wowserver/internal/application/worldlogin"\n'
    )
    text = replace_once(text, old_imports, new_imports, "worldd.go import block")
    text = replace_once(
        text,
        "Characters: chars,",
        "Characters: charlist.NewService(chars),",
        "worldd.go Characters wiring",
    )
    text = replace_once(
        text,
        "h.Creator = charapp.NewService(profile, creator)",
        "h.Creator = charcreate.NewService(profile, creator)",
        "worldd.go Creator wiring",
    )
    text = replace_once(
        text,
        "h.Players = chars",
        "h.Players = worldlogin.NewService(chars)",
        "worldd.go Players wiring",
    )
    return text


def main() -> int:
    import argparse

    ap = argparse.ArgumentParser(
        description="Slice 4: application use-case layer + phase-10 domain anchors"
    )
    ap.add_argument("root", nargs="?", default=".", help="wowserver repo root")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    if not root.is_dir():
        print("error: %s is not a directory" % root)
        return 2

    create_dir = root / NEW_CREATE
    create_tbc_dir = root / NEW_TBC_PROFILE
    list_file = root / "internal" / "application" / "character" / "list" / "service.go"
    worldlogin_file = root / "internal" / "application" / "worldlogin" / "service.go"
    old_service = root / OLD_APPCHAR / "service.go"
    old_profile_dir = root / OLD_TBC_PROFILE

    if (
        (create_dir / "service.go").exists()
        and list_file.exists()
        and worldlogin_file.exists()
        and not old_service.exists()
        and not (old_profile_dir / "creation.go").exists()
    ):
        print("slice 4 already applied (create/list/worldlogin present, old tree gone)")
        write_text(root / "docs" / "architecture.md", ARCHITECTURE_MD, False)
        print("refreshed docs/architecture.md")
        return 0

    errors: list[str] = []
    staged_write: dict[Path, tuple[str | None, str, bool]] = {}
    staged_delete: list[Path] = []

    # Repo-wide CRLF convention, used for brand-new Go files.
    repo_crlf = False
    probe = root / "internal" / "config" / "config.go"
    if probe.exists():
        repo_crlf = read_text(probe)[1]

    # ---- 1. character creation use case into create/ ----------------------
    if old_service.exists():
        original, crlf = read_text(old_service)
        for anchor in ("type Service struct {", "func NewService(profile CreationProfile, store CreationStore) *Service {"):
            if anchor not in original:
                errors.append("application/character/service.go drifted (missing: %s)" % anchor)
        if not errors:
            staged_write[create_dir / "service.go"] = (None, adapt_create_service(original), crlf)
            staged_delete.append(old_service)
    elif not (create_dir / "service.go").exists():
        errors.append("missing internal/application/character/service.go")

    # ---- 2. creation profile directory into create/tbc/ -------------------
    if old_profile_dir.is_dir():
        payloads = sorted(
            p for p in old_profile_dir.iterdir()
            if p.is_file() and not p.name.endswith(BAK_SUFFIX)
        )
        if payloads:
            for src in payloads:
                dst = create_tbc_dir / src.name
                if src.suffix == ".go":
                    original, crlf = read_text(src)
                    if "package tbc" not in original:
                        errors.append("%s does not declare 'package tbc'" % src.name)
                        continue
                    staged_write[dst] = (None, adapt_moved_profile(original), crlf)
                else:
                    # testdata and friends travel with the package
                    staged_write[dst] = (None, src.read_bytes().decode("utf-8", errors="surrogateescape"), False)
                staged_delete.append(src)
        elif not create_tbc_dir.exists():
            errors.append("missing internal/application/character/tbc/ (no movable files and no destination)")
        # else: only .skel.bak backups remain and the destination exists;
        # the move already happened in a previous (aborted) attempt.
    elif not create_tbc_dir.exists():
        errors.append("missing internal/application/character/tbc/")

    # ---- 3. composition root rewiring --------------------------------------
    worldd = root / "internal" / "app" / "worldd.go"
    if worldd.exists():
        original, crlf = read_text(worldd)
        try:
            patched = patch_worldd(original)
            staged_write[worldd] = (original, patched, crlf)
        except PatchError as e:
            errors.append("internal/app/worldd.go: %s" % e)
    else:
        errors.append("missing internal/app/worldd.go")

    # ---- 4. new use cases and anchors ---------------------------------------
    char_dir = root / "internal" / "application" / "character"
    staged_write[char_dir / "compat.go"] = (None, COMPAT_GO, repo_crlf)
    staged_write[list_file] = (None, LIST_SERVICE_GO, repo_crlf)
    staged_write[worldlogin_file] = (None, WORLDLOGIN_SERVICE_GO, repo_crlf)
    staged_write[root / "internal" / "application" / "auth" / "doc.go"] = (None, APP_AUTH_DOC_GO, repo_crlf)
    staged_write[root / "internal" / "application" / "chat" / "doc.go"] = (None, APP_CHAT_DOC_GO, repo_crlf)
    staged_write[root / "internal" / "application" / "movement" / "doc.go"] = (None, APP_MOVEMENT_DOC_GO, repo_crlf)
    staged_write[root / "internal" / "domain" / "world" / "doc.go"] = (None, DOMAIN_WORLD_DOC_GO, repo_crlf)
    staged_write[root / "internal" / "protocol" / "world" / "tbc" / "routes" / "doc.go"] = (None, ROUTES_DOC_GO, repo_crlf)

    # ---- 5. tree-wide import rewrite for the moved profile path -------------
    if not errors:
        for path in collect_go_files(root):
            if path in staged_write or path in staged_delete:
                continue  # staged content is already final
            try:
                original, crlf = read_text(path)
            except OSError:
                continue
            if OLD_PROFILE_IMPORT not in original:
                continue
            staged_write[path] = (original, original.replace(OLD_PROFILE_IMPORT, NEW_PROFILE_IMPORT), crlf)

    staged_write[root / "docs" / "architecture.md"] = (None, ARCHITECTURE_MD, False)

    if errors:
        print("\nNOTHING WAS CHANGED. Problems:\n")
        for e in errors:
            print(" - " + e)
        return 1

    for path, (original, new, crlf) in staged_write.items():
        if isinstance(new, str) and original == new:
            continue
        if original is not None and path.exists():
            backup_once(path)
        write_text(path, new, crlf)
        print("%s    %s" % ("wrote" if original is None else "patched", path))

    for path in staged_delete:
        if not path.exists():
            continue
        backup_once(path)
        path.unlink()
        print("removed  %s" % path)
        remove_empty_dir(path.parent)

    if old_profile_dir.exists() and not any(old_profile_dir.rglob("*")):
        shutil.rmtree(old_profile_dir, ignore_errors=True)
        print("removed  %s/" % old_profile_dir)

    print(
        "\nDone. From the repo root run:\n"
        "  go build ./...\n"
        "  go vet ./...\n"
        "  go test ./internal/...\n"
        "  python scripts/check_imports.py\n"
        "\nNotes:\n"
        "  * application/character/compat.go is temporary (slice 5 removes it).\n"
        "  * White-box tests of the moved profile keep working: the package name\n"
        "    is still tbc, only the import path changed.\n"
        "  * Old dirs that now hold only .skel.bak backups (internal/account/,\n"
        "    internal/realm/, internal/application/character/tbc/) are not part\n"
        "    of the build; delete them whenever you like.\n"
        "Backups use suffix %s" % BAK_SUFFIX
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
