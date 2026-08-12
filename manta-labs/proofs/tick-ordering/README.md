# Manta tick ordering (combat vs entities)

## Purpose

Prove that for a real `.dem`, manta callbacks satisfy:

1. **Non-decreasing `p.Tick`** across combat-log and entity callbacks.
2. **Cross-tick barrier:** after any combat-log callback at tick `T`, no later entity callback has `tick < T`.  
   Equivalently: **all entity updates for tick `N` finish before the first combat log for tick `N+1`.**
3. **Same-tick priority:** when a tick has both combat and entities, combat is observed before the first entity callback on that tick.

This underpins lasthits `closePendingHeroDamageBeforeTick` (safe to finalize tick `N` pending when tick `N+1` combat starts).

## Reproduce

```bash
./manta-labs/proofs/tick-ordering/run.sh
# or:
REPLAY=path/to.dem ./manta-labs/proofs/tick-ordering/run.sh
```

## Expected output

`manta-labs/lasthits-debug/examples/tick-ordering.txt` (Warlock `8915936762`):

```text
violations_tick_decreased	0
violations_entity_after_later_combat	0
same_tick_entity_before_first_combat	0
RESULT	PASS
```

Also verified on PA `8934466456` (`tick-ordering-pa.txt`).

## Why (manta internals)

- Outer demo messages are read in file order; each sets `p.Tick` then dispatches (`manta-master/parser.go`).
- Within one `CDemoPacket`, messages are stable-sorted by priority: combat (0) before `svc_PacketEntities` (5) (`manta-master/demo_packet.go`).
