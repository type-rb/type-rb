# Package and compiler integration boundaries

Use when deciding where a new API or capability belongs.

1. Use an ordinary TypeRB package by default.
2. Use an explicit platform package for backend- or ecosystem-specific
   behavior.
3. Add an API to `trb/std/*` only when it is foundational, portable, broadly
   applicable, and stable enough to version with the compiler.
4. Treat bundling as a distribution decision. It never grants compiler
   privileges or makes a dependency implicit.
5. Use compiler integration only when TypeRB source, native dependencies, and
   the current extension protocol cannot express the required behavior. Do not
   choose it merely because a package is official or convenient to ship.
6. When compiler integration is unavoidable, document the missing extension
   capability and preserve a path toward an ordinary package.
