# Product Source Of Truth

This reference defines how `herald-autopilot` should stay grounded in product intent so agents do not drift into feature guessing based on screenshots, local code shape, or whatever happens to be easiest to infer.

## Canonical Product Layers

Use these layers in order:

1. `VISION.md`
   Product direction, user-visible intent, roadmap state, and what should exist at the feature level.
2. `ARCHITECTURE.md`
   High-level boundaries, responsibilities, contracts, data flow, and phase intent.
3. `docs/superpowers/specs/*.md`
   Concrete feature-level specs and design decisions for specific work.

Treat these as the product-definition source of truth.

## Supporting But Non-Canonical Inputs

These matter, but they are not the product-definition source of truth:

- current code
- current screenshots
- current runtime behavior
- run artifacts and reports

They are evidence of what exists, not authoritative proof of what the product should become.

## Decision Rules

### New feature or visible behavior change

Ground the work in docs first:

- update the relevant acceptance or test-plan docs when required by repo policy
- update `VISION.md`
- update `ARCHITECTURE.md` if the change affects boundaries, responsibilities, or data flow
- add or update a spec under `docs/superpowers/specs/` when the change is non-trivial
- then implement

### Bug fix

If the intended behavior is already clear in product docs, fix the bug against that definition.

If the intended behavior is ambiguous:

- reconcile the docs first
- or create a focused spec that defines the intended behavior
- then implement

### Pure refactor or internal cleanup

You do not need to update product-definition docs unless the change also alters visible behavior or architecture commitments.

## GEPA Integration

When GEPA is optimizing the workflow, it should prefer changes that improve grounding against the product-definition layers. Good signals include:

- fewer feature guesses that required later correction
- more runs that updated the right source docs before code
- clearer specs for implementation workers
- less reliance on screenshots as a substitute for product intent

## Practical Rule

When working on product behavior, ask:

"What do `VISION.md`, `ARCHITECTURE.md`, and the relevant spec say?"

Only after answering that should the workflow ask:

"What does the current code do?"
