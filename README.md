# WoW TBC in Go — An Educational

This project is a gradual reimplementation of parts of the **World of Warcraft: The Burning Crusade 2.4.3, build 8606** server in Go; for learning Go, networking, binary protocols, databases, and software architecture, and the joy of seeing the result in a real client.

The primary reference for behavior and data structures is **[MaNGOS One](https://github.com/mangosone/server)**. This project is independent and unofficial and makes no claim to fully implement MaNGOS, be production-ready, or be officially affiliated with Blizzard or the MaNGOS team.

Currently, authentication, Realm List, character list, creation of all race/class combinations present in `playercreateinfo`, login of existing characters, and local movement are implemented. Character logout includes a 20-second countdown, freeze (root + sit), and immediate logout within the areatrigger_tavern range (tavern/rest areas). The **broadcast system and world simulation** — including spawn/despawn (CREATE_OBJECT/DESTROY_OBJECT), visibility-based broadcast, movement echo, NAME_QUERY for other players, SAY/YELL/EMOTE chat broadcast, **chat channels (CMSG_JOIN_CHANNEL/LEAVE_CHANNEL/Say)**, whisper to other players, and 1-to-1 (SendTo) — are implemented. Stat calculations and the combat/spell/quest sections are still being completed.

**Single project document:** [PROJECT.md](docs/PROJECT.md) — architecture, setup, actual feature status, differences from the reference, limitations, verification evidence, and next steps.

## Quick Start

Prerequisites: Go per `go.mod` (minimum 1.24), compatible databases containing MaNGOS One data, DBC output from the 2.4.3 client, and a client of the same build. SQL containing only the schema is not sufficient to run the world.

From the project root, if you haven't created the config files yet:

```powershell
Copy-Item configs/realmd.conf.example configs/realmd.conf
Copy-Item configs/worldd.conf.example configs/worldd.conf
```

Edit the database specifications in the files and place the DBCs in `dbc/`. The samples are for a local development environment; adapt their specifications to your environment.

Two separate terminals:

```powershell
go run ./cmd/realmd
```

```powershell
go run ./cmd/worldd
```

Local client: `set realmlist 127.0.0.1`. The Realm record in the database must also point to the `worldd` address and port, typically `127.0.0.1:8085`.

Management tool:

```powershell
go run ./cmd/wowctl account check
go run ./cmd/wowctl realm list
go run ./cmd/wowctl account create testgo ExamplePassword123
```

Creating an account writes data; the check command is for verifying connectivity and structure. The database guide and the limitations on running servers concurrently are covered in the comprehensive document.

## Reference and Attribution

- Primary reference: [mangosone/server](https://github.com/mangosone/server).
- Pinned reference for opcodes and parts of the protocol: [`b2083578a279aac3a70a2cba8fe710a454324c4e`](https://github.com/mangosone/server/tree/b2083578a279aac3a70a2cba8fe710a454324c4e).
- Preserved reference files and generated outputs carry copyright notices and `GPL-3.0-or-later`; these notices have been retained. The [upstream license](https://github.com/mangosone/server/blob/master/LICENSE) and the exact origin of the files are noted in the comprehensive document.
- Names, trademarks, and content of World of Warcraft belong to their respective owners, including Blizzard Entertainment. This repository does not claim ownership of them.
- DBC, client, MPQ, and game data are not considered authored output of this project. This README does not create redistribution rights for them; presenting the project as educational does not substitute for the terms of the source licenses.

No independent license has been designated for all project code in this change; attributing the reference alone does not mean all distribution requirements are resolved. Reference files and existing notices must not be removed upon distribution.
