# Comprehensive Project Document and Differences from MaNGOS One

Last summary: **2026-11-04**. This document replaces scattered documentation and the previous phase roadmap. Development from now on is based on **domain and use case**, not phase numbers.

"Implemented" means the code path exists; "client-confirmed" means the user has reported that behavior. Neither alone equals full compatibility or testing of all failure modes. During this documentation consolidation round, no testing, build, service run, or database change was performed.

## Table of Contents

1. [Goal and Reference](#goal-and-reference)
2. [Status and Differences](#status-and-differences)
3. [Architecture and Ownership](#architecture-and-ownership)
4. [Setup and Configuration](#setup-and-configuration)
5. [Database and Transactions](#database-and-transactions)
6. [Static Data](#static-data)
7. [Network Contract and Login](#network-contract-and-login)
8. [Movement, Logout, and Stats](#movement-logout-and-stats)
9. [Validation and Troubleshooting](#validation-and-troubleshooting)
10. [Next Steps](#next-steps)
11. [Code Origin and Generation](#code-origin-and-generation)

## Goal and Reference

An educational and fun project for learning Go and gradually rebuilding the client TBC login-to-world path. It is not a complete replacement for the C++ server or a ready public server.

| Source                                                   | Contract                                                  |
| -------------------------------------------------------- | --------------------------------------------------------- |
| [MaNGOS One](https://github.com/mangosone/server)        | Behavior and protocol 2.4.3 / build 8606                  |
| commit `b2083578a279aac3a70a2cba8fe710a454324c4e`        | Pinned reference for opcode inputs and part of the port   |
| `mangos/realmd@095bc797730669f228d5126f4627b7d2a2e13a8f` | Recorded historical reference for auth                    |
| submitted dump `all.sql` and existing databases          | Actual MaNGOS schema; MySQL 5.6.51 in the dev environment |
| DBC extracted with MaNGOS tooling                        | Client 2.4.3 data; no new extractor needed                |

In recent reviews, the adjacent local source `../server` and upstream pages were also read. The adjacent source has local changes and full equality with the pinned commit is not claimed. For subsequent ports, the file/function and reference revision must be specified.

## Status and Differences

This section is a **behavioral diff from the reference**, not a line-by-line Git diff.

| Domain                                   | What Exists                                                                                                                                                                                                                                                                                                                                                                                                       | Limitation or Remaining Work                                                                                                                                                                                              |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Auth                                     | SRP6, session key storage, reconnect, account/IP ban, locked IP, failed_logins                                                                                                                                                                                                                                                                                                                                    | MFA, Warden, CRC, autoban, and separation of all reference error codes are absent                                                                                                                                         |
| Password error                           | build 8606 error response and limited wait for the client to close the connection                                                                                                                                                                                                                                                                                                                                 | Correct error display in the client after the fix is not yet independently confirmed                                                                                                                                      |
| Realm                                    | List, character count, access, build, local/public address                                                                                                                                                                                                                                                                                                                                                        | Population and world status are not automatically managed; incompatible realm is removed                                                                                                                                  |
| World session                            | auth digest, header crypt, addon response, ping, bounded queue, FSM                                                                                                                                                                                                                                                                                                                                               | Admission is in-process only; session replacement and multi-process coordination are incomplete                                                                                                                           |
| Character list                           | Account-based, appearance, equipment from cache, pet for the selection screen                                                                                                                                                                                                                                                                                                                                     | Cache is not rebuilt; corrupt data may be dropped from output with a warning                                                                                                                                              |
| Character create/delete                  | All race/class pairs present in `playercreateinfo`, both genders, starting data, and full transaction; character delete with hard delete of all dependent tables (character_action/aura/homebind/inventory/queststatus/reputation/skills/spell/spell_cooldown, corpse, character_pet + pet_aura/spell/cooldown, item_instance) in one transaction; `realmcharacters.numchars` decremented in the same transaction | Name is ASCII only; race/class restriction is taken from the catalog, not hardcoded; multi-effect spell machine is still incomplete                                                                                       |
| Existing character login                 | Valid race/class in data, both genders, level 1–70, map present in DBC                                                                                                                                                                                                                                                                                                                                            | This does not mean full mechanic support for all classes, instances, taxi, death/resurrection, or forms                                                                                                                   |
| Inventory on login                       | Items, bags, contents, bank, keyring, and self update                                                                                                                                                                                                                                                                                                                                                             | Equip/move/use operations and inventory change persistence are not implemented                                                                                                                                            |
| Movement                                 | Forward/back, strafe, turn, walk/run, jump/land, and heartbeat                                                                                                                                                                                                                                                                                                                                                    | Local; no broadcast, collision, terrain, speed validation, or position persistence                                                                                                                                        |
| Logout                                   | 20-second countdown with root + sit (partial UPDATE_OBJECT + SMSG_STANDSTATE_UPDATE + SMSG_FORCE_MOVE_ROOT), cancel with unroot + stand, save time/tutorial, release online, completion response; **immediate logout in areatrigger_tavern (rest areas)** with loading of `areatrigger_tavern` and `AreaTrigger.dbc`                                                                                              | Instant logout in capital city (REST_TYPE_IN_CITY) requires AreaTable.dbc and AREA_FLAG_CAPITAL; taxi/GM instant logout not yet implemented; UNIT_FLAG_STUNNED is deliberately not set so the equipped pipeline completes |
| Stats                                    | Base + equipment + full aura modifiers (passive spells reapplied at login like Player::addSpell, item equip spells and SPELL enchantments as passive holders, buffs/talents through the aura pipeline): HP/Mana, armor/resistances (flat + percent), AP/RAP (flat + percent), stat percent, combat ratings incl. SPELL_AURA_MOD-rating masks, derived crit/dodge/parry/block/expertise/spell-crit percentages; cooldowns are millisecond-exact and expire-pruned                                                                                                                               | Item set bonuses, enchantment procs, spell/healing power coefficients, and ammo bonus remain; forms do not swap the power bar; characters without a stored Defense skill are treated as fully skilled                                                                   |
| Query/UI                                 | self name, item template, time, played time, some settings and chat messages (SAY/YELL/EMOTE/WHISPER/PARTY/GUILD/RAID/CHANNEL with echo to sender)                                                                                                                                                                                                                                                                | social/ticket are currently placeholders; mail/raid/guild broadcast and many queries are without a service; whisper only to self and otherwise gives player-not-found                                                     |
| guild and many queries without a service |                                                                                                                                                                                                                                                                                                                                                                                                                   |                                                                                                                                                                                                                           |
| Shared World                             | Broadcast registry + presence with team/faction template, spawn/destroy and visibility streaming of creatures/gameobjects, random creature movement, melee/spell combat against creatures, creature combat window (UNIT_FLAG_IN_COMBAT), corpse decay/respawn with auras removed on death, and out-of-combat creature health regeneration (maxHealth/3 per 2 s) | Player-versus-player combat: live combat-state registry with cross-session event inboxes (melee swings and hostile spells on red-flagged opposing-team players, DoTs ticking on the victim session, interrupt effects with school lockout, kill credit logged), player death (auras removed, release timer) with release spirit (ghost aura 8326/night-elf wisp, PLAYER_FLAGS_GHOST persisted through relogin), graveyard repop at the homebind and spirit-healer resurrection (50% + resurrection sickness 15007 by level); UNIT_FLAG_IN_COMBAT windows for fighting players; public unit-field/combat-log broadcast so other players see regen, buffs, casts and damage; CMSG_SETSHEATHED with virtual weapon slots; merged per-creature aura display shared by all attackers. AI, threat, chase, quest, loot, duels, honor and resurrection by player spells do not exist |
| Management                               | `wowctl account create/check` and `realm list/add`                                                                                                                                                                                                                                                                                                                                                                | We have no automatic migration or `account init`                                                                                                                                                                          |
| Other versions                           | Version-scoped protocol/DBC/persistence boundaries                                                                                                                                                                                                                                                                                                                                                                | Only 8606 is active; automatic compatibility with Classic/WotLK is not claimed                                                                                                                                            |

Important difference: **character creation** now supports all pairs present in `playercreateinfo`, not just Human Warrior. Taxi mask follows `InitTaxiNodesForLevel` for each race, and skill/language spells are extracted from SkillLineAbility with the race/class mask.

### More Precise Differences

- Realm must have build 8606 in `realmbuilds`. Version filtering is not the same as displaying the version family as offline in the reference. Subnet matching is done for IPv4; DNS can only provide the output address.
- No custom schema like `auth_accounts` or a `client_build` column has been added to account. Account locale/os is not updated from login.
- The creation limit is 10 characters per realm and 50 per account; name and PvP policy apply to GM too. Realm icons 1 and 6 are considered in the faction rule.
- New name: 2 to 12 English letters, no three identical consecutive letters; Title Case, reserved check, and duplicate check. Profanity and locale rules are incomplete.
- Startup is strictly strict about missing essential data; fallback and gap-filling of reference levels are not implemented.
- addon: limited decompression, max 256 addons and 255-byte name; malformed input is rejected.
- World handshake error is mostly generic; overspeed ping, realm capacity queue, and Warden do not exist.
- Auth account is limited concurrently in the same world process; the C++ server should not write to the shared character DB at the same time.

## Architecture and Ownership

Pattern: light layering with ports/adapters; use case logic is separated from packet and SQL. We do not have heavy DDD, a general event bus, or a goroutine per opcode.

```text
cmd/
  realmd/                     auth and Realm List entrypoint
  worldd/                     world entrypoint
  wowctl/                     management tool
internal/
  app/                        composition roots and resource lifecycle
  config/                     file, env, CLI, and validation
  domain/
    account/ realm/ character/ world/
  application/
    character/create/         creation service; tbc subpackage for initializer
    character/list/           account list
    character/stats/          stat snapshot calculation
    worldlogin/               acquire/release player
    movement/                 apply local position
    world/                    World/Runner/Intent/Event/Outbox contracts
    auth/ chat/               currently placeholder
  protocol/
    auth/tbc/                 SRP orchestration, reconnect, realms, codec
    client/ wire/             version profile and binary primitives
    world/tbc/
      frame/ crypt/ codec/    framing, header cipher, pure payloads
      opcodes/ updatefields/  generated constants
      routes/                 future route location; currently placeholder
  storage/
    realmd/ characters/ world/ sqlutil/
    characters/mangosone8606/ item_instance format
  gamedata/                   immutable Catalog and tbc version loader
  dbc/                        WDBC file parser
  auth/srp/                   transport-independent SRP library
  transport/                  TCP lifecycle, deadline, cancellation, capacity
  world/                      simulation ownership contract; no loop yet
  admin/ testclient/          management tool and test helper
configs/                      conf samples; real file ignored
tools/                        generators and schema preparation
tools/reference/mangos/       reference snapshots and manifest hash
scripts/                      import boundary checks
docker/                       prepared local SQL; dumps ignored
dbc/                          local client data; ignored
docs/PROJECT.md               comprehensive document
```

Dependency rules:

- domain does not depend on protocol/storage/application/world/app; it holds models and pure rules.
- application does not depend on storage/protocol/world/app; interfaces are defined on the consumer side.
- protocol must not import SQL or storage; decode → use case → encode.
- storage must not import protocol. gamedata consumption by storage/world and the shared SRP library are deliberate exceptions.
- `app` wires dependencies; cmd only handles parse, signal, and exit.
- The future world only owns simulation; communication with session will be via mailbox/Intent/Event. Contracts are not yet implementation.

Actual status: auth orchestration is still in protocol; `application/auth` has no extracted service. Some load limitations are still inside the storage transaction. `application/character/compat.go` and `protocol/world/tbc/split_compat.go` remain for compatibility. Some tests are still under `internal/character`. These are cleanup backlog.

Each connection has one reader, one writer, and one session loop. Reader/Writer each own their cipher state. Only the session loop changes player state and snapshot. `Handler.active` with mutex is only account admission, not game state ownership. Queues are bounded; shutdown closes the connection and waits for workers.

FSM: `Authed → LoggingIn → InWorld → LoggingOut → Authed`; login failure returns to Authed and logout cancel returns to InWorld. Any live state can become Closed. The external RequestTransition callback must receive a definitive result; it is not called from the owning loop itself.

## Setup and Configuration

Commands run from the repository root. `go.mod` specifies minimum Go 1.24; Node is only needed for generators. A MySQL compatible with the schema and actual world data is required. The previous MySQL 5.6.51 experience only confirms the tested paths, not guaranteed compatibility of all features.

Real files:

- `configs/realmd.conf` from the `.example` sample; default listen `127.0.0.1:3724`.
- `configs/worldd.conf` from the `.example` sample; default listen `127.0.0.1:8085`, realm_id=1.
- The `.env` file is not loaded automatically. Process envs still override.
- Precedence: default → file → environment → CLI. `WOW_AUTH_DSN` directly overrides the login DSN; during troubleshooting check leftover shell variables.

| conf section            | main keys                                                            |
| ----------------------- | -------------------------------------------------------------------- |
| `[network]`             | listen, max_connections, auth_timeout, idle_timeout, request_timeout |
| `[database.login]`      | host, port, name, user, password                                     |
| `[database.characters]` | same keys; realm character database                                  |
| `[database.world]`      | same keys; static world database                                     |
| `[world]`               | realm_id, send_queue                                                 |
| `[data]`                | dbc_path, dbc_locale, load_timeout                                   |
| `[logging]`             | level: debug / info / warn / error                                   |

Local sample: login=`realmd`, characters=`character1`, world=`mangos1` on 3306. Adapt credentials from the sample to your environment; this document does not store real environment passwords. realmd only needs login; worldd consumes all three. send_queue is between 1 and 1024; dbc_locale is between 0 and 15, DBC path relative to working directory.

Available general envs: `WOW_LISTEN`, `WOW_MAX_CONNECTIONS`, `WOW_AUTH_TIMEOUT`, `WOW_IDLE_TIMEOUT`, `WOW_REQUEST_TIMEOUT`, `WOW_LOG_LEVEL`, `WOW_DB_HOST/PORT/NAME/USER/PASSWORD`, and `WOW_AUTH_DSN`. The old DB group relates to login; keep characters/world settings in conf. The general listen variable can affect both services.

```powershell
go run ./cmd/realmd -config configs/realmd.conf
go run ./cmd/worldd -config configs/worldd.conf
go run ./cmd/wowctl account check
go run ./cmd/wowctl realm list
go run ./cmd/wowctl account create testgo ExamplePassword123
go run ./cmd/wowctl realm add Local 127.0.0.1 8085
```

The two services run in separate terminals. `wowctl realmd` is not a valid command; old cmd/account and cmd/realm wrappers do not exist in the current tree. account create does not change a duplicate account, and realm add is a write operation. Before add, view the existing realm list. The realm record address must be reachable by the client; localhost is only for the same machine.

## Database and Transactions

The schema owner is MaNGOS; there is no Go migration or CREATE TABLE in the service path. Each realm has a CharacterDSN and `characters` has no realm_id column. For a second realm, an appropriate connection is needed.

| Database   | Consumption                                                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------------- |
| realmd     | account, bans, sessionkey, realmlist, realmcharacters                                                         |
| character1 | characters, homebind, inventory/item_instance, spells/actions/skills/reputation/tutorial and enumeration data |
| mangos1    | playercreateinfo\*, level/class stats, XP, item_template, creature_template, reserved_name                    |

`sha_pass_hash` comes from uppercase name/password; s/v/sessionkey are stored in the reference numeric hex form and the adapter converts them to fixed-width little-endian bytes. Accounts with empty/short s/v can be populated from the existing hash at login. Keys and password must not be logged.

Character creation: cap/name/faction policy is in domain, but the facts and policy execution **remain inside the lock and transaction**. GET_LOCK on a dedicated connection, account lock, and GUID allocation prevent races between Go processes. login and characters must be on the same endpoint and the DB user needs cross-schema access. Written tables must be InnoDB.

The creation transaction includes characters/homebind, item_instance/inventory, spell/action/skills/reputation, and realmcharacters. INSERTs are batched and success is sent only after commit. An unknown commit outcome is not automatically retried. The user-reported delay after batching went from about 6.8 seconds to about 677 milliseconds; the benchmark is not guaranteed.

Login: ownership is checked by account/guid, online flag, and data under transaction, and the player is reserved. Normal logout/disconnect: online=0, logout_time, played time, tutorial, clearing first-login, and position (position_x/y/z, orientation, map, zone) are saved in the same transaction; position is only written when the player has entered the world. **Inventory, health, money, and changed spells are still not overwritten from an incomplete snapshot.**

After a crash, online=1 may remain. Startup does not do a global reset. There are two recovery paths: (1) `wowctl account character reset-online [GUID]` which manually clears the online flag (no GUID = all), and (2) AcquirePlayer, if logout_time is older than the idle timeout, treats it as stale and reclaims it (for single-process; multi-process needs external coordination). The C++ server should not write alongside Go on the shared character DB; it does not honor the Go contractual lock.

### Independent Docker

`compose.yaml` creates the `wow-mangos-sandbox` environment with profile=`sandbox` and MySQL 5.6.51 on **127.0.0.1:3308**. This environment differs from the main 3306 database. The data volume is preserved; init SQL only runs on a fresh volume.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/prepare-schema.ps1 -DumpPath C:\path\all.sql
docker compose --profile sandbox up -d --wait mangos-db
```

The initial dump had only schema; real mangos1 data and DBC are needed for world startup. The generated SQL file in docker/init is local and ignored. Volume deletion or database reset commands are not part of normal setup.

## Static Data

The Catalog is read-only after full load at startup. The world DB is not consumed for static queries during gameplay; changing data requires restart. MyISAM snapshot tables share no transaction, so static data must not be edited during load.

### Entity model (MaNGOS-one parity)

World-database entities live in `internal/entity` as field-for-field ports of the reference C++ structs: `CreatureInfo` (all 75 creature_template columns), `CreatureData` (creature spawns), `GameObjectInfo` (gameobject_template with the data0..data23 block), `GameObjectData`, the full `ItemPrototype` (item_template, 135 columns incl. spellppmRate), `Quest` (quest_template, 127 columns plus computed objective counts), vendor/trainer/gossip/npc_text/page_text entities, area triggers, graveyards, exploration XP, pet data, game events, conditions, game_tele, POIs, reputation templates and the player-creation tables. `entity.Dataset` mirrors ObjectMgr's storages (mCreatureDataMap, sGOStorage, sItemStorage, mQuestTemplates, ...).

### Loader registry (Open/Closed)

`internal/worldload` loads every table family. Each loader is a self-registering unit in its own file (`init()` + `Register`), runs in the MaNGOS `World::SetInitialWorldSettings` order (order constants are spaced by 100 so new loaders slot in without renumbering), and follows the reference's validation semantics: outErrorDb-style warnings with skip-and-continue, capped at a few lines per table. Adding a new entity family means adding one file; the runner, progress and startup wiring never change.

Loaded families (MaNGOS order): mangos_string, spell_template, instance_template, page_text, gameobject_template, npc_text, item_template, creature_model_info, creature_equip_template (+raw), creature_template_classlevelstats (split per expansion like m_creatureClassLvlStats), creature_template, creature_template_spells, creature_model_race, item_required_target, reputation tables, points_of_interest, petcreateinfo_spell, creature (spawns, LEFT JOINed with game_event/pool tables like LoadCreatures), creature_addon + creature_template_addon, gameobject (spawns, same JOINs), quest_template, quest_relations (split into creature/GO starter/ender maps), conditions, areatrigger_teleport, areatrigger_tavern, game_graveyard_zone, the playercreateinfo family, exploration_basexp, pet_name_generation, pet_levelstats, mail_level_reward, skill_fishing_base_level, game_tele, game_event, npc_vendor(+template), npc_trainer(+template), gossip_menu(+option), reserved_name.

Not yet ported (registry makes each a one-file addition): locales_* tables, loot templates, db_scripts, waypoint/pathing tables, pool_template internals, character-side entities that MaNGOS loads from the characters DB.

### Live startup progress

The runner counts every table up front (`SELECT COUNT(*)`, the BarGoLink budget) and reports a row-weighted percentage while each table streams: `creature_template 45% (23451/52134 rows)`, mapped onto the 20..60 band of the worldd startup. Per-table completion lines (`loaded entity table=creature_template rows=52134 skipped=3 took_ms=812`) and capped validation warnings go to the startup log — the counterpart of MaNGOS `>> Loaded N ...` and outErrorDb output. No step silently blocks the console.

### Catalog and spawn resolution

`gamedata.Catalog` consumes the dataset once and exposes typed lookups (`CreatureTemplate`, `GameObjectTemplate`, `ItemPrototype`, `Quest`, `CreatureModelInfo`, `CreatureClassLvlStats`, spawn queries returning GUID+data pairs). `ResolveCreatureSpawn` ports `Creature::SelectLevel`: level drawn from the template MinLevel..MaxLevel range (deterministically from the spawn GUID where the reference uses urand, so every startup shows the same world), health/mana from creature_template_classlevelstats x template multipliers with the legacy Min/MaxLevelHealth interpolation fallback, modelid_override honored, bounding radius/combat reach from creature_model_info, speeds from template rates over base movement speeds (walk 2.5, run 7.0). The CREATE_OBJECT codec consumes the resolved spawn directly.

Main DBCs: ChrRaces, ChrClasses, CharBaseInfo, CharStartOutfit, Map, Spell, Item, SpellItemEnchantment, Faction. The creation initializer also reads CharSection, CharacterFacialHairStyles, SkillLine, and SkillLineAbility.

Item display is corrected from Item.dbc; the database inventory type is preserved. The item_template row with entry=0 is skipped with a warning and counter; other essential data must not be replaced with fabrications.

## Network Contract and Login

| World direction | Header  | Length                             | Opcode               | Body cap    |
| --------------- | ------- | ---------------------------------- | -------------------- | ----------- |
| client → server | 6 bytes | uint16 big-endian, includes opcode | uint32 little-endian | 10236 bytes |
| server → client | 4 bytes | uint16 big-endian, includes opcode | uint16 little-endian | 65533 bytes |

Only the header is encrypted. AuthCrypt TBC uses HMAC-SHA1 derivation and stateful XOR/add/subtract, not RC4. Trailing zeros of the little-endian key representation are removed before HMAC per reference BigNumber behavior. The reader must not read-ahead across the crypt activation boundary. After an I/O error or cipher state consumption, retrying the same packet is not allowed.

`wire.Reader` is bounded and sticky-error; the decoder must check Finish. Queue payload must not be mutated concurrently. Framing cap, known opcode, and state permission are three separate controls.

### General Order

1. realmd: challenge/proof → sessionkey publication → realm list or reconnect.
2. worldd: AUTH_CHALLENGE → AUTH_SESSION → digest/build/ban/realm check → crypt → AUTH_RESPONSE and addon.
3. character screen: CHAR_ENUM and optional settings; then PLAYER_LOGIN with full GUID.
4. Session check and player acquire → stat calculation → build the full packet set → send → InWorld and time sync.
5. Query, movement, ping, and logout requests.

Current server login packets in order: DUNGEON_DIFFICULTY, LOGIN_VERIFY_WORLD, ACCOUNT_DATA_TIMES, FEATURE_SYSTEM_STATUS, MOTD, SET_REST_START, BINDPOINTUPDATE, TUTORIAL_FLAGS, INITIAL_SPELLS, SEND_UNLEARN_SPELLS, ACTION_BUTTONS, INITIALIZE_FACTIONS, LOGIN_SETTIMESPEED, CLIENT_CONTROL_UPDATE, UPDATE_OBJECT for the player/items, INIT_WORLD_STATES, and TIME_SYNC_REQ. The SMSG/MSG prefix follows the generated table.

The mandatory login ROOT has been removed. Cinematic, full social push, guild/group updates, login spell, aura restoration, item duration, and reference zone world-state are still incomplete. Account-data timestamps and some lists are empty placeholders. The order of all client packets is not fixed and varies with cache, addon, and character state.

### Router and Side Opcodes

- VOICE_SESSION_ENABLE two bytes, OPT_OUT_OF_LOOT four bytes, TAXI_BENCHMARK one byte, and ACTIVE_VOICE_CHANNEL a uint32 + CString are consumed; no success response or voice service is built.
- NAME_QUERY only responds for self after load; non-self/pre-login request may go unanswered. ITEM_QUERY_SINGLE returns the template.
- CONTACT_LIST and GMTICKET_GETTICKET have placeholder responses. MAIL_TIME, RAID_INFO, BATTLEFIELD_STATUS, MEETINGSTONE, guild bank, and channels have no complete service.
- A known client/bidirectional opcode without a handler is only set aside in allowed state metadata and gives one warning per connection/opcode. This is merely keeping the connection alive, not implementing the feature; UI waiting for a response may wait forever.
- Unknown opcode, server-only, STATUS_NEVER, and disallowed state are still errors. The existing handler strictly decodes the payload.
- After a successful logout in Authed, late LOGGEDIN and LOGGEDIN_OR_RECENTLY_LOGGEDOUT messages are set aside; UPDATE_ACCOUNT_DATA has its own path. This exception is not active before the first login. The next successful login clears the recent logout marker.

## Movement, Logout, and Stats

### Local Movement

The codec checks the 8606 MovementInfo including the jump/pitch section and dependent tails. application/movement applies bounded and finite position to the session snapshot; orientation is normalized. Transport and flight flags are out of scope. Walk/run, turn, strafe, jump/land, heartbeat, and pitch/swim transition have routes.

The client displays its own movement; there is no broadcast to others. The numeric coordinate limitation is not a substitute for terrain, collision, map admission, anti-cheat, or fall damage. Position is saved during logout in the same transaction (only when the player has entered the world), so the next login is from the previous location. Validation is in application/movement and storage only writes the snapshot; terrain, collision, and map admission do not yet exist.

### Logout

Normal logout has a twenty-second countdown; time sync/ping continue during it. At countdown start, the character goes to sit via partial SMSG_UPDATE_OBJECT on UNIT_FIELD_BYTES_1, a separate SMSG_STANDSTATE_UPDATE is sent, and the player is frozen with SMSG_FORCE_MOVE_ROOT so they cannot move. On cancel (manual or by movement), stand + unroot is done again. Movement or jump cancels the countdown; heartbeat without movement does not cancel. **Immediate logout**: if the player is within the `areatrigger_tavern` range (PLAYER_FLAGS_RESTING set), they are saved and released immediately without countdown and without freeze, and SMSG_LOGOUT_RESPONSE with `instantLogout=1` and then SMSG_LOGOUT_COMPLETE is sent. Tavern entry detection is done with the CMSG_AREATRIGGER handler and `IsPointInAreaTriggerZone` (radius or oriented-box). The player is checked via a tick every 250ms, and if they leave the range, rest is cleared. After save/release, LOGOUT_COMPLETE is sent and state returns to Authed. After that, no handler should access the released player.

Instant logout in capital city (REST_TYPE_IN_CITY) is not yet implemented because it requires `AreaTable.dbc` and `AREA_FLAG_CAPITAL`. Taxi and GM instant logout are also outside the current scope.

Two successive fixes were made: first only late LOGGEDIN was covered; a new log showed CANCEL_TRADE comes from LOGGEDIN_OR_RECENTLY_LOGGEDOUT. The last fix covers both. In the logout → relogin cycle stabilization, non-critical errors (send failure for LOGOUT_COMPLETE and time sync, temporary DB error in release, late packet with player=nil) no longer cause disconnect; release is retried on the next tick and handlers prevent panic with explicit nil-check. A real session change error (key different from lookup) still disconnects.

### Stat Calculation

Path: `application/character/stats.Calculate` → `domain/character.Stats` → codec update fields. Reference: StatSystem.cpp, PlayerItemApply.cpp, PlayerItemEnchant.cpp, PlayerStats.cpp, and DBCStructure.h.

- Five attributes from race/class/level plus healthy equipment bonus; backpack and bank items have no effect. Broken equipment is set aside.
- HP: base class health + direct item HP + first 20 stamina at coefficient 1 and excess at coefficient 10.
- Mana: base class mana + direct item mana + first 20 intellect at coefficient 1 and excess at coefficient 15; class without mana does not get this bonus.
- Armor from equipment + twice agility; school resistances and shield block value are also calculated.
- AP has a class/level/strength/agility dependent formula; damage comes from weapon template and `max(AP,0)/14 × attack seconds`. Offhand coefficient 0.5 and unarmed base 1–2 with 2-second time.
- Enchant/gem without condition applies direct stat, resistance, or weapon damage effect; proc on hit has no idle effect. Item stat kinds 12..37 (ITEM_MOD_*_RATING) sum into the PLAYER_FIELD_COMBAT_RATING_* block exactly like Player::_ApplyItemBonuses; combined kinds (hit, crit, resilience, haste) feed several ratings at once. The client converts hit/expertise/resilience ratings to percentages itself via gtCombatRatings.
- Melee/ranged/offhand crit, dodge, parry, block and per-school spell crit percentages are derived like StatSystem.cpp: agility/intellect game tables plus defense/weapon-skill differences and the rating bonuses; parry/block add the 5% base only for classes that know the ability (a learned spell with the parry/block spell effect). PLAYER_EXPERTISE is the CR_EXPERTISE rating bonus. The melee and spell rolls use the same hit/crit/expertise rating bonuses.
- Damage min/max, speed, AP, positive/negative stat bonus, armor/resistance, and block fields are sent explicitly. Current HP comes from DB and is clamped to the calculated maximum for output, not full-healed.

**This is not the full reference stat engine.** Passive/racial spell, talent, stored buff, form, equip-spell, set bonus, conditional meta-gem, random suffix scaling, mana regen, spell/healing power, and ammo bonus remain, and no aura modifies the new percentage fields yet. Therefore an equipped or buffed character may still have different HP/damage. The login warning announces limitations; a claim of full equality should not be inferred from it.

## Validation and Troubleshooting

### Existing Evidence

| Area                                             | Evidence and validity limit                                                                                                                                 |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| auth, realm, world handshake, and list           | User has reported successful passage with the real client                                                                                                   |
| Character creation (all race/class)              | Code exists; go build/vet/test green; client confirmation pending (especially Horde races and Blood Elf/Draenei)                                            |
| Logout: root+sit+countdown and instant in tavern | Code exists; build/vet/race tests green; client confirmation pending (especially instant logout in inn and cancel behavior via movement)                    |
| Historical rearrangement before world login      | Previous report of race/vet/build and successful race/rollback test; user also confirmed the refactor                                                       |
| World load and other characters                  | User has confirmed login with other heroes; correctness of all stats not confirmed                                                                          |
| Movement                                         | Code added; we have no explicit report covering all movements in recorded evidence                                                                          |
| Last logout fix + relogin cycle stabilization    | Code exists; retry of temporary DB error in release, non-critical send for LOGOUT_COMPLETE/time sync, nil-check in handlers; re-client confirmation pending |
| New stats                                        | Code exists; test/build/client verification not performed                                                                                                   |

A green result of old tests is not generalized to new changes. The following commands were run in this round and were green (date 2026-11-04, three patches: creation / logout root+sit / instant logout in tavern):

```powershell
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
```

Integrations are opt-in: `WOW_TEST_DSN` for sandbox auth, `WOW_CREATION_TEST_DSN` for creation on 3308 and temporary schemas, `WOW_EXISTING_DSN` for read testing of the existing database, and `WOW_STATIC_TEST_DSN` for reading static data. A missing variable means skip, not SQL success. Set the real DSN value and credential in the local shell; do not direct write tests to the main DB. The current static test builds the DBC path with the old relative path; after package moves, its path must be fixed before relying on this integration. The `areatrigger_tavern` data in the world DB is required for instant logout to be active; a missing table does not fall back to normal countdown, only `Taverns` in the summary will be zero and instant logout will not be active.

Historical tests included SRP/crypt vectors, incomplete/sticky framing, timeout/cancellation, state transition, policy, name and cap race, and mid-insert/realm count rollback. For recent logout/stat/movement/bag changes, fresh coverage is needed. `scripts/check_imports.py` currently also scans external-package tests and may consider the allowed test dependency on protocol a production violation; this separation must be fixed.

### Useful Logs

`[logging] level=debug` for packet and stage; info for main events, warn for limitations. Debug can be disabled and sensitive auth payload must not be printed.

| Sign                               | Check                                                                                                                      |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Listener does not come up          | config, DB connection, DBC, and last startup error                                                                         |
| static data: invalid/missing       | complete dataset, file build, structure, and locale; entry=0 item is only a warning                                        |
| character list omitted             | warning GUID, race/class, and name/coordinates/equipmentCache                                                              |
| player login rejected              | ownership, remaining online, at_login, and inventory/homebind data                                                         |
| unsupported opcode                 | name, number, state, and metadata; do not confuse missing handler with corrupt payload                                     |
| failure after logout               | check both late statuses; no handler should consume player=nil                                                             |
| world feature not implemented      | packet set aside; feature does not yet exist                                                                               |
| stat warning or different HP       | unapplied effects, gear, aura, and class/level data                                                                        |
| Location rolled back after relogin | movement is not yet saved in DB; known behavior                                                                            |
| Instant logout not active          | check `Taverns` in startup log; if zero, the `areatrigger_tavern` table in world DB is empty or AreaTrigger.dbc is missing |

## Next Steps

**Suggested priority: client confirmation of the three new patches (creation of all race/classes, logout with root+sit, instant logout in tavern) and then completing stats and adding city rest.**

1. **Client confirmation of the three recent patches**:
   - Create a character of every race/class present in `playercreateinfo` (especially Horde and Blood Elf/Draenei) and check taxi mask, language skills, and outfit.
   - Enter an inn (e.g., Goldshire Inn) and confirm the `PLAYER_FLAGS_RESTING` pickup in the client (the "ZZZ" icon in the portrait) and instant logout.
   - Confirm root+sit during countdown and cancel with movement.
2. **City rest (REST_TYPE_IN_CITY)**: load `AreaTable.dbc` and check `AREA_FLAG_CAPITAL` on each zone change; this also enables instant logout in capital cities. Requires a zone-update pipeline.
3. Align stat calculations with controlled samples in MaNGOS: naked, weapon, stamina/intellect gear, broken equipment, enchant, mana/rage/energy class, and level 70. First correctness of the existing path, then expand modifiers.
4. Stats/spells: the aura modifier pipeline (passives, equip spells, buffs) has landed; next are spell/healing power coefficients for casts, item set bonuses (needs ItemSet data), enchantment procs, and the shapeshift power-bar swap (stances/forms display only today). Unknown data must remain visible, not claimed as a complete result.
5. World/movement: add real simulation ownership, map/position validation, and confirmed position save in the logout transaction; cross-session combat uses the pvp registry inbox pattern as the template for further shared state.
5a. Combat: creature AI/threat/chase so PvE can fight back, diminishing returns on PvP crowd control, honor/kill rewards, duels, and player-cast resurrection (SMSG_RESURRECT_REQUEST to another player).
5b. Death: load world_safe_locs for real graveyards and the corpse object; until then the homebind is the graveyard and the corpse is skipped.
6. Develop inventory operations (equip/move/use) and character rename each as an independent use case.
7. Social/query/mail/instance, spell/combat/AI/quest, and other versions are followed after a stable foundation.

End criterion for the suggested stage: several logins/logouts without unexpected disconnect, no touching of another account's character, release of online, explainable stat display relative to the reference, and clear recording of still-unsupported effects. These criteria are not yet fully met.

## Code Origin and Generation

The inputs `tools/reference/mangos/Opcodes.h` and `OpcodeTable.cpp` are snapshots of the pinned reference; their own copyright and GPL notice is preserved. They are generator inputs, not a second C++ server. `manifest.json` keeps their SHA-256 hashes and the generator verifies match before generation; original line endings are preserved with `.gitattributes`.

```powershell
node tools/generate-opcodes.mjs
node tools/generate-opcodes.mjs --check
node tools/generate-update-fields.mjs C:\path\to\pinned\UpdateFields.h
```

Generated opcode metadata does not by itself grant handler permission. Update fields are also 8606-specific and must not be reused for another version. Generators should be consumed instead of editing output by hand. Changing the reference requires simultaneous review of commit, source, manifest, and protocol assumptions.

Main references: [AuthCrypt](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/shared/Auth/AuthCrypt.cpp), [OpcodeTable](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Server/OpcodeTable.cpp), [CharacterHandler](https://github.com/mangosone/server/blob/master/src/game/WorldHandlers/CharacterHandler.cpp), [StatSystem](https://github.com/mangosone/server/blob/master/src/game/Object/StatSystem.cpp), [upstream license](https://github.com/mangosone/server/blob/master/LICENSE).

The previous local README inside tools/reference has been merged into this section; the source snapshot itself, manifest, and license notices are not removed. This document does not create a new license for the reference code, client, or Blizzard data. The root README keeps the project introduction and source attribution; this file is the sole reference for technical details and backlog.
