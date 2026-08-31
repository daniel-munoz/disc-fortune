<!--
A PR here is usually one roadmap phase, and always downstream of a design doc
and a plan. Those carry the detail; this body is for what a reviewer needs
before they start reading the diff.

Delete any section that does not apply. An empty heading helps nobody.
-->

## What changed for the user

<!--
Lead with behaviour, not implementation. If an existing default changed, say so
here rather than further down: someone scripting against the old behaviour
should learn it in the first paragraph, along with the flag that restores it.
-->

## Design and plan

<!-- Links into docs/plans/. -->

## What to review hardest

<!--
The one or two places you are least sure of, or where the change is subtle
enough that a reviewer skimming the diff would miss it. Naming a weak spot is
worth more than a summary of the strong ones.
-->

## Verification

<!--
The commands you ran and what they said. If you exercised the real binary
rather than only the suite, say what you ran and what you observed -- a
behaviour claim backed by an actual run is worth more than a green check.
-->

## Decisions later phases inherit

<!--
Anything a future task has to know that the code does not say on its own: a
constraint that must not be broken, an invariant a call site depends on, a
trade-off deliberately accepted. The roadmap records these per phase; surface
them here too so a reviewer can push back while it is still cheap.
-->
