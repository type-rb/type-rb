# Native QBE variant

`backend.json` pins the QBE 1.3 release archive and all five patch digests.
`python3 tools/qbe_toolchain.py /path/to/qbe-1.3` verifies them, applies the
patches without fuzz, builds QBE and checks supported-target instruction emission.
QBE itself is compiled with explicit `-O2`, without fast-math or host-specific
architecture flags. The binary's receipt records upstream, patch, builder,
C compiler and exact CFLAGS identities.
Checkout builds include those tracked inputs in their content cache. An explicit
`TRBN_QBE` override must support the same operations, ARM64 operand selection
and expression-motion and Float-contraction behavior.

The six added upstream lines define a unary single/double floating-point `sqrt`
and select `fsqrt`, `sqrtss` or `sqrtsd`. Native uses the double form only for
the independently verified standard `Math.sqrt` declaration. Other declarations and existing callable paths retain the ordinary call ABI.
Directly taking a standard module function as a value remains unsupported by
Native; an ordinary wrapper function can be passed as a callable.
There is no constant folding, fast-math mode or new runtime helper. Native does
not expose libm `errno`; observable floating-point results remain the contract.

The external C driver is a test, not compiler or runtime implementation. It
compares both scalar widths with libm under four rounding modes, including
signed zero, subnormals, infinities, NaNs and deterministic random bit patterns.
Run `python3 tools/qbe_sqrt_test.py --qbe /path/to/qbe` on arm64 and amd64.
RV64 and Windows emission are checked; those are not supported Native runtimes.

`arm64-immediates.patch` retains encodable integer constants in the second
operand of addition, subtraction, bitwise operations and shifts. It reuses QBE's
immediate classifiers, normalizes word constants and reduces shift counts modulo
the result width as required by the IL. Unsupported constants, variable operands,
addresses and floating-point operations keep ordinary register selection. Logical
masks use their own printing rule instead of the arithmetic `lsl #12` suffix.
Run `python3 tools/qbe_integer_test.py --qbe /path/to/qbe` for an independent
fixed-width arithmetic oracle, signed and unsigned boundaries, oversized shift
counts and fallback controls. AMD64 tests pass negative and oversized shift
counts through registers because its upstream emitter does not normalize them
to assembler immediates; ARM64 tests retain these immediate cases.
`--baseline /path/to/previous/qbe` also compares
unchanged AMD64 emission. The patch adds no target-independent optimization or
runtime helper.

`arm64-memory.patch` folds an address addition into each eligible load or store
when a register base and offset fit the instruction. Offsets can be positive,
naturally aligned constants or 64-bit registers, optionally shifted by the
element width. Shared address and shift definitions keep their remaining uses;
only the final folded use removes a definition. For a shared address, folding
requires a natural-width shift so it saves both address operations; shared
constant and unscaled addresses remain explicit. It uses the existing
memory operand representation and shared liveness, spilling and register
allocation. Non-memory uses of an address, frame and symbol addresses and
unsupported constant displacements retain the ordinary path. Other shifts remain explicit
and can supply an unscaled register offset. Address definitions are inspected
before instruction storage is replaced. No memory operation moves or changes width.
Run `python3 tools/qbe_memory_test.py --qbe /path/to/qbe` for byte-level load/store
checks, signed extension, floating-point bit preservation, displacement bounds,
fallbacks, wrapping indexes, shared loads and stores, escaped addresses, shared
shifts, and enough live bases and indexes
to require spilling across calls. Its optional
`--baseline` control compares AMD64 emission with the preceding backend.

`gcm.patch` fixes recursive expression sinking to update the emitted instruction
instead of its temporary copy. Otherwise the copied root still refers to the
original operands and leaves the newly sunk children unused. It also retains a
compound branch predicate outside a more frequently executed loop, using QBE's
existing loop weights. Single comparisons remain eligible for rematerialization,
so instruction selection can use branch flags without retaining a Boolean value.
Cheap memory addresses retain the same policy to limit register pressure. This
prevents invariant predicate trees from being recomputed on each visit.
Before scheduling, a worklist marks phi and copy dependencies from every
non-copy operand and terminator. Unmarked copies, including otherwise unused
phi cycles, are removed. The pass keeps all ordinary operations and leaves
their elimination to the existing scheduler. Calls, memory operands, return
values and loop conditions remain roots of this analysis. Functions without
phi nodes do not need the pass.
The scheduler visits pinned instruction groups in their original order before
remaining pure expressions. GVN may rescue a shared expression from a folded
branch into the entry block; recursively scheduling that expression first could
otherwise pull a later call or load ahead of an earlier side effect. The ordered
call probe also rejects a cached backend lacking this repair.
The change adds no instruction, alias assumption or
arithmetic rewrite, and applies to every target.
Run `python3 tools/qbe_gcm_test.py --qbe /path/to/qbe`
for six-target emission checks and an independent native oracle for shared
predicates, signed boundaries, zero-trip loops, invariant memory addresses,
unused copy cycles, cycles observed through calls, stores and returns, and
call/load ordering after constant branch folding.
Unlike the ARM64 selector patches alone, this patch can change AMD64 emission.

`fma.patch` adds the floating-point `mulc` operation: ordinary multiplication
whose use in addition or subtraction may be contracted to one rounding.
Plain `mul` retains separate rounding. On ARM64, a late emission check combines
an adjacent `mulc` and `add`/`sub` only when the product register is overwritten
by the consumer, overwritten before any further read in that block, or dead
at its exit. Functions containing `mulc` reuse QBE's liveness analysis after
allocation, including inserted copies and stack-slot writes, to check successor
uses. ABI return registers remain implicit uses, and calls stop the local proof.
For a predecessor-fed addend, the exit-liveness extension permits contraction
only when a multiplication input is directly produced by a local division or
square root. This lets the update combine a late product with an available
addend while keeping short load/multiply reduction chains separate. The search
uses the most recent register definition and stops at calls; it changes only
profitability, not the product-observation proof.
Other targets and unproved pairs emit
ordinary multiplication. The patch does not add a third operand to generic QBE
instructions, alter register allocation, reassociate expressions, or assume
finite values. Independently observed products retain their rounded value.
Native emits `mulc` only for verified Float multiplication, including operands
already widened from Integer. Checked Integer multiplication is unchanged.
Run `python3 tools/qbe_fma_test.py --qbe /path/to/qbe` for both scalar widths,
six-target emission, a libm `fma` oracle, strict multiplication, signed zero,
subnormals, nonfinite values, random bit patterns, product reuse, calls and
branches, successor observations and loop-carried products.
`--baseline /path/to/previous/qbe` verifies rejection of the new IL.

Maintenance is five pinned patches, one acquisition helper and target/edge tests.
A QBE upgrade must rebase the patches, run upstream tests, run these controls on
each supported host, and repeat ordinary compiler validation. Historical seed
manifests and their replay workflows retain their original stock QBE identity;
the current compatibility schema records this variant separately. No patched
binary is mislabeled as an upstream release.
