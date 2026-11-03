# World login: protocol coverage and remaining work

Domain: `application/worldlogin`; build adapter: `protocol/world/tbc` (8606).
This is a source audit, not a packet capture or a successful client-login claim.
No tests were written/run and no build/server was executed for this change.

## Reference and method

- All `STATUS_AUTHED` entries were read from the vendored
  `tools/reference/mangos/OpcodeTable.cpp` (commit
  `b2083578a279aac3a70a2cba8fe710a454324c4e`). The generated Go catalogue uses it.
- [WorldSession.cpp](https://github.com/mangosone/server/blob/master/src/game/Server/WorldSession.cpp):
  dispatch distinguishes unhandled/disallowed commands from malformed payloads.
- [CharacterHandler.cpp](https://github.com/mangosone/server/blob/master/src/game/WorldHandlers/CharacterHandler.cpp):
  `HandlePlayerLoginOpcode`, character loading and map admission.
- [Player.cpp](https://github.com/mangosone/server/blob/master/src/game/Object/Player.cpp):
  `SendInitialPacketsBeforeAddToMap` and `SendInitialPacketsAfterAddToMap`.
- [GroupHandler.cpp](https://github.com/mangosone/server/blob/master/src/game/WorldHandlers/GroupHandler.cpp),
  `VoiceChatHandler.cpp`, `MiscHandler.cpp`, and `LFGHandler.cpp`: preferences.

GitHub pages and the local sibling `../server` were used for handler inspection;
the sibling has local changes and is not asserted to equal the pinned commit.
The exact pre-login permissions come from the vendored pinned opcode table.
Client cache, addons, social state, first login, guild, corpse, taxi and auras
change the exchange. Server source gives handlers and server ordering, not a
single exhaustive chronological client packet trace.

## Before character entry: complete STATUS_AUTHED inventory

These commands are permitted after world-session authentication in MaNGOS;
that does not mean every one is sent on every login.

| Client opcode(s) | Go handling / limitation |
|---|---|
| `CMSG_CHAR_ENUM`, `CMSG_CHAR_CREATE` | Implemented; character selection state only |
| `CMSG_CHAR_DELETE` | Explicit disabled response |
| `CMSG_PLAYER_LOGIN` | Ownership/session checks, acquire, initial packets |
| `CMSG_NAME_QUERY` | Self response after loading; pre-login/non-self requests currently have no answer |
| `CMSG_GUILD_QUERY` | Unsupported feature, logged and discarded |
| `CMSG_PING`, `CMSG_KEEP_ALIVE` | Pong / no reply |
| `CMSG_SET_ACTIONBAR_TOGGLES` | One-byte validated preference; not persisted |
| `CMSG_CHAR_RENAME` | Unsupported; no success response, no mutation |
| `CMSG_WARDEN_DATA` | Unsupported; no Warden service |
| `CMSG_LFG_SET_AUTOJOIN`, `CMSG_LFM_SET_AUTOFILL` | Unsupported LFG service; logged and discarded |
| `CMSG_SET_TAXI_BENCHMARK_MODE` | One-byte preference; consumed without reply |
| `CMSG_REALM_SPLIT` | Implemented response |
| `CMSG_VOICE_SESSION_ENABLE` | Two one-byte preferences; no voice service/reply |
| `CMSG_SET_ACTIVE_VOICE_CHANNEL` | uint32 + bounded CString; no service/reply |
| `CMSG_OPT_OUT_OF_LOOT` | uint32; consumed without reply, as reference (activation unimplemented) |
| `CMSG_SET_PLAYER_DECLINED_NAMES` | Unsupported; no persistence |

## Server initialization sequence

The Go adapter currently emits the following ordered sequence after acquisition:

1. `MSG_SET_DUNGEON_DIFFICULTY`, `SMSG_LOGIN_VERIFY_WORLD`.
2. `SMSG_ACCOUNT_DATA_TIMES`, `SMSG_FEATURE_SYSTEM_STATUS`, `SMSG_MOTD`.
3. `SMSG_SET_REST_START`, `SMSG_BINDPOINTUPDATE`, `SMSG_TUTORIAL_FLAGS`.
4. `SMSG_INITIAL_SPELLS`, `SMSG_SEND_UNLEARN_SPELLS`, `SMSG_ACTION_BUTTONS`,
   `SMSG_INITIALIZE_FACTIONS`, `SMSG_LOGIN_SETTIMESPEED`.
5. `SMSG_CLIENT_CONTROL_UPDATE`, `SMSG_UPDATE_OBJECT` (self and carried items).
6. `SMSG_INIT_WORLD_STATES`, `SMSG_FORCE_MOVE_ROOT`, `SMSG_TIME_SYNC_REQ`.

The reference additionally performs real map admission and visibility, sends
social data proactively, and conditionally sends guild data, cinematic,
corpse/reclaim data, group updates, login spell/aura effects, enchant/item
duration updates, and notifications. Our root is a deliberate stationary-world
restriction, not a universal MaNGOS login requirement. World states currently
have an empty field list; account-data timestamps are zero, cooldown lists are
empty, and there is no aura/stat simulation. Merely sending opcode names is not
full semantic compatibility.

## Requests after loading

| Family | Current handling |
|---|---|
| `CMSG_SET_ACTIVE_MOVER`, `CMSG_TIME_SYNC_RESP`, root ACK | Validated, implemented |
| `CMSG_ITEM_QUERY_SINGLE`, `CMSG_NAME_QUERY` | Template query / self-only name query |
| `CMSG_QUERY_TIME`, `CMSG_PLAYED_TIME` | Implemented, daily reset time is a placeholder |
| `CMSG_CONTACT_LIST`, `CMSG_GMTICKET_GETTICKET` | Existing empty-list/no-ticket placeholders, not DB-backed services |
| `CMSG_REQUEST_ACCOUNT_DATA`, `CMSG_UPDATE_ACCOUNT_DATA` | Consumed; no account-data sync service |
| Tutorials | Changed in session and saved on release |
| Zone, selection, stand state, action/faction UI changes | Consumed; no corresponding simulation/persistence |
| Movement | Selected movement reports parsed; position not applied |
| `CMSG_GMTICKET_SYSTEMSTATUS`, `MSG_QUERY_NEXT_MAIL_TIME`, `CMSG_REQUEST_RAID_INFO`, `CMSG_MEETINGSTONE_INFO` | Missing responses; known commands are logged/discarded in allowed states |
| Guild, channels/chat, quest queries, creature/gameobject queries | Missing services; not necessary to pretend these are implemented to keep the connection open |
| Logout request/cancel | Countdown, save/release, completion |

## Dispatch policy

- Registered routes retain explicit state checks and strict payload decoders.
- Unregistered **known client/bidirectional** commands can be discarded only
  when their pinned metadata permits the current state. A warning is emitted
  once per opcode per connection; debug receive logs still show each packet.
- No handler, database write, state change, or fabricated success reply occurs
  on this fallback. Only framing has been validated; unsupported payloads are
  deliberately not semantically parsed.
- Unknown opcodes, server-only commands, `STATUS_NEVER`, unsupported transfer
  commands, and state violations remain errors. Handshake cannot be replayed
  through the fallback. This is stricter than MaNGOS's log-and-skip policy for
  some disallowed packets.

This prevents an optional missing feature from tearing down an otherwise valid
login. It does NOT implement that feature: a client UI awaiting its response may
still wait. Missing responses above remain explicit backlog, grouped by domain.

## Next work by domain

1. Worldlogin: validate the actual initial update payload and admission with a
   real client; complete missing required responses discovered from that trace.
2. Character/query: non-self name lookups and persistent social data.
3. Account settings / support / mail / instances: replace documented placeholders
   and missing responses with domain services and real database state.
4. World/movement: map ownership, visibility, movement, aura/stat simulation.

Existing restrictions remain: living level-1 Human Warrior on map 0, stationary
world, limited supported inventory and pending-login flags. No change to DB
schema, acquisition atomicity or save/release behavior was made in this audit.
