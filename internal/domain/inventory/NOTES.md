# inventory — port notes

Ported from MaNGOS One: PlayerItem*.cpp, ItemHandler.cpp (SWAP_INV_ITEM,
SWAP_ITEM, AUTOEQUIP_ITEM, AUTOEQUIP_ITEM_SLOT, SPLIT_ITEM, DESTROYITEM).

Skipped on purpose (documented differences):
- Bank: positions are valid, but storing/banking returns EQUIP_ERR_TOO_FAR_AWAY_FROM_BANK.
- Item stats / sets / enchant durations / on-store spells: `Hooks` are called at the
  reference points (`_ApplyItemMods`, `RemoveItemDependentAurasAndCasts`) but are no-ops until the stat patch.
- Weapon-switch cooldown (m_weaponChangeTimer) and the SMSG_SPELL_COOLDOWN it sends.
- Gems in CanEquipUniqueItem (HasItemOrGemWithIdEquipped only checks the items themselves).
- IsBindedNotWith / temporary loot checks (every carried item is owned by the player; no loot yet).
- Reputation requirement uses PlayerState.ReputationRank when set (currently not wired).
- Off-hand that cannot be stored when a 2H weapon is equipped (AutoUnequipOffhandIfNeed else-branch)
  is removed instead of mailed.
- Other players are not told about equipment changes yet (visible-item values go to the owner only).
