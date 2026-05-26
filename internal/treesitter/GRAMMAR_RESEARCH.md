# Grammar Go-Binding Research

Generated during Dream spec gap audit. Documents the Go-binding availability
for all 16 languages listed in `runtime_language_exceptions.v1.json`.

## Summary

| #  | Language    | Go Module Available | Binding Type           | Compatibility | Status                    |
|----|-------------|---------------------|------------------------|---------------|---------------------------|
| 1  | gleam       | yes                 | go-tree-sitter         | compatible    | DROP-IN                   |
| 2  | ql          | yes                 | go-tree-sitter         | compatible    | DROP-IN                   |
| 3  | udev        | yes                 | go-tree-sitter         | compatible    | DROP-IN                   |
| 4  | elixir      | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 5  | elm         | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 6  | kotlin      | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 7  | matlab      | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 8  | r           | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 9  | racket      | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 10 | swift       | yes                 | go-tree-sitter         | compatible    | PATH-MISMATCH             |
| 11 | commonlisp  | yes                 | smacker/go-tree-sitter | incompatible  | PATH-MISMATCH + LEGACY    |
| 12 | elisp       | yes                 | smacker/go-tree-sitter | incompatible  | PATH-MISMATCH + LEGACY    |
| 13 | solidity    | yes                 | smacker/go-tree-sitter | incompatible  | PATH-MISMATCH + LEGACY    |
| 14 | zig         | yes                 | smacker/go-tree-sitter | incompatible  | PATH-MISMATCH + LEGACY    |
| 15 | d           | no                  | N/A                    | N/A           | BLOCKED                   |
| 16 | fortran     | no                  | N/A                    | N/A           | BLOCKED                   |

## Categories

### Drop-In (3)

These grammars use `github.com/tree-sitter/go-tree-sitter` v0.24+, have Go bindings
at `bindings/go/`, and their module paths follow the standard convention. Adding them
requires only: `go get`, import, register in `parser.go`, remove from exceptions JSON.

- **gleam**, **ql**, **udev**

### Path-Mismatch — Modern Binding (7)

These grammars have Go bindings using the compatible `go-tree-sitter` binding but
their GitHub owner/repo paths differ from what might be expected. They require
`go get` with the correct import path and registration in `parser.go`.

- **elixir**, **elm**, **kotlin**, **matlab**, **r**, **racket**, **swift**

### Path-Mismatch — Legacy Binding (4)

These grammars use the older `github.com/smacker/go-tree-sitter` binding which is
incompatible with the current `github.com/tree-sitter/go-tree-sitter` v0.25.0 used
by Crush. They require either:

- Migration to a fork using the modern binding, or
- Wrapping the legacy binding with a compatibility shim

- **commonlisp**, **elisp**, **solidity**, **zig**

### Blocked (2)

No usable Go bindings exist for these languages. They cannot be added until bindings
are created upstream.

- **d** — No Go binding published
- **fortran** — No Go binding published

## Per-Language Details

### gleam

- **GitHub**: `tree-sitter-grammars/tree-sitter-gleam`
- **Import**: `github.com/tree-sitter-grammars/tree-sitter-gleam/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Standard tree-sitter-grammars org. Drop-in.

### ql

- **GitHub**: `tree-sitter/tree-sitter-ql`
- **Import**: `github.com/tree-sitter/tree-sitter-ql/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Official tree-sitter org. Drop-in.

### udev

- **GitHub**: `tree-sitter-grammars/tree-sitter-udev`
- **Import**: `github.com/tree-sitter-grammars/tree-sitter-udev/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Standard tree-sitter-grammars org. Drop-in.

### elixir

- **GitHub**: `elixir-lang/tree-sitter-elixir`
- **Import**: `github.com/elixir-lang/tree-sitter-elixir/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Maintained by Elixir lang team.

### elm

- **GitHub**: `elm-tooling/tree-sitter-elm`
- **Import**: `github.com/elm-tooling/tree-sitter-elm/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Uses proper v5.x semver tags (NOT +incompatible).

### kotlin

- **GitHub**: `tree-sitter-grammars/tree-sitter-kotlin` (recommended)
- **Alt**: `fwcd/tree-sitter-kotlin` (original, less maintained)
- **Import**: `github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Use tree-sitter-grammars fork — better maintained.

### matlab

- **GitHub**: `tree-sitter-grammars/tree-sitter-matlab`
- **Import**: `github.com/tree-sitter-grammars/tree-sitter-matlab/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)

### r

- **GitHub**: `r-lib/tree-sitter-r`
- **Import**: `github.com/r-lib/tree-sitter-r/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Maintained by R project.

### racket

- **GitHub**: `tree-sitter-grammars/tree-sitter-racket`
- **Import**: `github.com/tree-sitter-grammars/tree-sitter-racket/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)

### swift

- **GitHub**: `alex-pinkus/tree-sitter-swift`
- **Import**: `github.com/alex-pinkus/tree-sitter-swift/bindings/go`
- **Binding**: `github.com/tree-sitter/go-tree-sitter` (compatible)
- **Notes**: Community-maintained, actively updated.

### commonlisp

- **GitHub**: `theHamsta/tree-sitter-commonlisp`
- **Import**: `github.com/theHamsta/tree-sitter-commonlisp/bindings/go`
- **Binding**: `github.com/smacker/go-tree-sitter` (**LEGACY — incompatible**)
- **Notes**: Uses old smacker binding. Needs migration or compatibility wrapper.

### elisp

- **GitHub**: `Wilfred/tree-sitter-elisp`
- **Import**: `github.com/Wilfred/tree-sitter-elisp/bindings/go`
- **Binding**: `github.com/smacker/go-tree-sitter` (**LEGACY — incompatible**)
- **Notes**: Uses old smacker binding. Needs migration or compatibility wrapper.

### solidity

- **GitHub**: `JoranHonig/tree-sitter-solidity`
- **Import**: `github.com/JoranHonig/tree-sitter-solidity/bindings/go`
- **Binding**: `github.com/smacker/go-tree-sitter` (**LEGACY — incompatible**)
- **Notes**: Uses old smacker binding. Needs migration or compatibility wrapper.

### zig

- **GitHub**: `maxxnino/tree-sitter-zig`
- **Import**: `github.com/maxxnino/tree-sitter-zig/bindings/go`
- **Binding**: `github.com/smacker/go-tree-sitter` (**LEGACY — incompatible**)
- **Notes**: Uses old smacker binding. Needs migration or compatibility wrapper.

### d

- **Status**: BLOCKED — no Go binding available
- **Notes**: No tree-sitter Go binding found for D language. Cannot be integrated
  until upstream creates one.

### fortran

- **Status**: BLOCKED — no Go binding available
- **Notes**: No tree-sitter Go binding found for Fortran. Cannot be integrated
  until upstream creates one.

## Integration Priority

1. **Tier 1 — Drop-In (3)**: gleam, ql, udev — minimal effort
2. **Tier 2 — Path-Mismatch Modern (7)**: elixir, elm, kotlin, matlab, r, racket, swift — moderate effort
3. **Tier 3 — Legacy Binding (4)**: commonlisp, elisp, solidity, zig — requires binding migration
4. **Blocked (2)**: d, fortran — not possible without upstream support
