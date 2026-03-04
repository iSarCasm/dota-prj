# Consolidated Notes

## TODO

- Cache the OpenDotaAPI response and cache downloaded replay files.
- Implement strategic states.

### Later

- Get better progress details from command output (possibly no longer needed if moved to service).

## Strategic States

1. Filter out pauses.
2. Death:
   - Health at 0.
3. Fighting:
   - Dealing or receiving damage to/from heroes.
   - Giving/receiving buffs from enemy heroes.
   - Giving buffs to heroes currently fighting.
4. Farming:
   - Lane farming: dealing damage to lane creeps.
   - Jungle farming: dealing damage to jungle creeps.
5. Roaming:
   - If nothing else happens for more than 10 seconds.

## Mistakes and Ratings

### Power Treads (PT) Switching

Positive:
- Switch PT to INT before using a spell.
- Show how much mana was saved.

Negative:
- Use spell without PT on INT.
- Show how much mana could have been saved.

Open question:
- Is it important that PT was switched back?

Sample scenarios:
- `mana = 900/1000; spell1 = 100; spell2 = 200`
- No PT switch: `900 - 100 = 800` (no saved mana)
- PT switched once:
  - `900 + 120*0.9 = 1008`
  - `1008 - 100 = 908` (8 saved mana)
- PT switched back:
  - `900 + 120*0.9 = 1008 (/1120)`
  - `1008 - 100 = 908 (/1120)`
  - `908/1120 * 1000 = 810` (10 saved mana)
- PT switched once (double spell):
  - `900 + 120*0.9 = 1008`
  - `1008 - 100 = 908`
  - `908 - 200 = 708` (8 saved mana)
- PT switched back (double spell):
  - `900 + 120*0.9 = 1008`
  - `1008 - 100 = 908`
  - `908 - 200 = 708`
  - `708/1120 * 1000 = 632` (32 saved mana)

Rules:
1. If PT is switched and mana is spent in the next 10s, show gained savings.
2. If PT is switched, mana is spent, and PT is switched back from INT within 10s, show gained savings.
3. If mana is spent without PT on INT, show missed savings.

Low mana stats:
1. Track mana dips between full mana states.
2. Track time spent below threshold (e.g. 20% / 50%).

Potential:
1. Show low-mana stats under perfect PT usage.

Notes:
1. No potential savings may mean PT was on INT all game.
2. INT heroes may be special case (bonus damage).
3. Could show missed HP opportunity: time not attacking/casting while taking damage.

Rating ideas:
1. All time on INT = perfect?
   - Not valid for many non-INT heroes.
   - Not always valid for INT heroes.
   - Consider universal heroes.
2. Always switching before spell = perfect?
3. `rating = mana_saved / potential_mana_saved`.

PT usage modes:
1. STR when damaged and not spending mana/attacking.
2. AGI when farming/damaging and not spending mana/being damaged.
3. INT when spending mana and not damaged.
4. MAIN ATTR when farming/damaging and not spending mana/being damaged.
5. NON-STR when healing and not being damaged or DPSing (STR heroes).
6. NON-INT when gaining mana and not using mana or DPSing (INT heroes).

General:
1. Ignore illusions.
2. Remove pause times.

PT-specific shorthand:
1. Mana ability used without PT on INT => minor mistake.
2. Switch to INT -> use mana ability -> switch to non-INT within 10s => good.

### Soul Ring

1. Use without ability after => minor mistake.
2. Use without PT on STR => minor mistake.

### BKB

1. Effect does not overlap with any PvP (deal/receive damage) => blunder.

### Midas

1. Utilization % (rating).

### Phase Boots

1. Utilization % (rating).

### Mango / Stick / Fairy

1. Using at full (99%) => medium mistake.

### Spell Missed

- No enemies affected.
- Examples: Gleipnir, Meteor Hammer (minor).

## Replay Parsing / Technical Notes

- `16777215` appears to be max for 3-byte `uint` (used as invalid/sentinel handle).
- Example handles observed:
  - `3574043 => CDOTA_Item_EmptyBottle`
  - `4246443 => CDOTA_Item_PowerTreads`
  - `13339623 => CDOTA_Item_MagicWand`
  - `838124 => CDOTA_Item_ManaDraught`
  - `5491033 => CDOTA_Item_Perseverance`
  - `6671105 => CDOTA_Item_Witch_Blade`
  - `9340920 => CDOTA_Item_BlinkDagger`
  - `5130626 => CDOTA_Item_TeleportScroll`
  - `4295331 => CDOTA_Item_Enhancement_Vampiric`
  - `7308954 => CDOTA_Item_Cyclone`
  - `15354432 => CDOTA_Item_UltimateOrb`

Power Treads entity observations:
- `m_iStat` likely tracks PT selected attribute:
  - `0 = STR`
  - `1 = INT`
  - `2 = AGI`
- `m_flAssembledTime` seems to update on every PT switch.

Potentially interesting callbacks:
- `onCUserMessageRequestInventory`
- `onCDOTAUserMsg_LocationPing`
- `onCDOTAUserMsg_ItemPurchased`
- `onCDOTAUserMsg_Ping`
- `onCDOTAUserMsg_ItemAlert`
- `onCDOTAUserMsg_AbilityPing`
- `onCDOTAUserMsg_CourierKilledAlert`
- `onCDOTAUserMsg_BuyBackStateAlert`
- `onCDOTAUserMsg_QuickBuyAlert`
- `onCDOTAUserMsg_HPManaAlert`
- `onCDOTAUserMsg_GlyphAlert`
- `onCDOTAUserMsg_XPAlert`
- `onCDOTAUserMsg_TipAlert`
- `onCDOTAUserMsg_TE_UnitAnimation`
- `onCDOTAUserMsg_TE_UnitAnimationEnd`
- `onCDOTAUserMsg_TE_Projectile`
- `onCDOTAUserMsg_TE_ProjectileLoc`

## Timing Fields Noted

- `m_pGameRules.m_flGameStartTime = 274.7667`
- `m_pGameRules.m_flHeroPickStateTransitionTime = 140.70001`
- `m_pGameRules.m_flPreGameStartTime = 184.76668`
