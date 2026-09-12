# Teach TypeRB interactively

When the user asks to learn TypeRB:

1. Establish the intended path: language basics, API/backend, database, Jobs,
   or browser UI.
2. Start with [A Tour of TypeRB](https://type-rb.github.io/tour/) or one small
   local `.trb` exercise that runs immediately.
3. Introduce one new concept at a time and let the learner write or modify the
   code before showing a complete answer unless they request it.
4. Validate the learner's actual code with `trb fmt` and `trb check`; explain
   the TypeRB rule behind each diagnostic.
5. End each step with one observable result and one suggested next exercise.

Prefer executable feedback over a long lecture. Link the relevant reference
section when the learner wants the full rule.
