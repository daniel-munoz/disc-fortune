<!--
This body is for what a reviewer needs before they start reading the diff.

Every section is optional. Delete the ones that do not apply -- an empty
heading helps nobody, and a one-line change deserves a one-line body. A PR
that fills all five is not better than one that fills two honestly.
-->

## What this changes

<!--
Lead with behaviour. If an existing default changed, say so here rather than
further down: someone scripting against the old behaviour should learn it in
the first paragraph, along with the flag or version that restores it.

If nothing user-facing changed -- a refactor, a dependency bump, a test-only
fix -- say that in one line and describe what did change instead. "No
behaviour change; this only moves X behind Y" is a complete answer, and it
tells a reviewer what kind of reading the diff needs.
-->

## Why

<!--
The reason this exists, in a sentence or two. A roadmap task can point at the
task; a bug fix should describe the symptom and how it was hit; a dependency
bump should say what forced it (a CVE, a build break, a version floor).

If the diff makes this obvious, delete the section.
-->

## Background

<!--
Links a reviewer may want: a design doc or plan under docs/plans/, an issue,
an upstream changelog or release note, the commit that introduced a bug.

Delete it when there is nothing to link. Not every change has a paper trail,
and inventing one is worse than having none.
-->

## What to review hardest

<!--
The one or two places you are least sure of, or where the change is subtle
enough that a reviewer skimming the diff would miss it. Naming a weak spot is
worth more than a summary of the strong ones.

"Nothing -- it is a typo fix" is a legitimate answer.
-->

## Verification

<!--
The commands you ran and what they said. If you exercised the real binary
rather than only the suite, say what you ran and what you observed -- a
behaviour claim backed by an actual run is worth more than a green check.

For a change that is hard to test, say how you convinced yourself instead,
and say what you could not check.
-->

## What this commits us to

<!--
Anything that outlives this PR and that the code does not say on its own: a
constraint that must not be broken, an invariant a call site now depends on, a
trade-off deliberately accepted, a public interface that is now hard to change.

Surface them here so a reviewer can push back while it is still cheap. A
dependency bump that raises a version floor belongs here too.

Delete it when the change commits us to nothing, which is most of the time.
-->
