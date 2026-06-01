# Usage

## Prerequisites

- **Go 1.23+** — [https://go.dev/dl](https://go.dev/dl)
- **Make** — pre-installed on macOS and most Linux distributions; on Windows use [GnuWin32](https://gnuwin32.sourceforge.net/packages/make.htm) or run `go run ./cmd/gen/` and `go test ./...` directly

Verify your Go version:
NO
```bash
go version  # should print go1.23 or higher
```

---

## Why Make?

Make was chosen as the developer tool because it gives the workflow a single, memorable entry point without requiring any additional dependencies beyond what Go already needs. A few specific reasons:

- **Discoverability** — `make generate`, `make test`, `make all` are self-describing. A new contributor doesn't need to read docs to know what each target does.
- **Dependency ordering** — Make lets targets depend on other targets. `test` depends on `generate`, so `make test` always regenerates the model before running the test suite. This prevents a common mistake where you run tests against a stale generated file.
- **No extra tooling** — alternatives like `Task` or `Mage` would require installing another binary. Make is pre-installed on macOS and Linux and needs no setup.
- **Convention** — Makefiles are a widely understood convention in Go projects. Reviewers and CI systems know how to use one without explanation.

If Make is not available (e.g. on Windows without a POSIX layer), the two underlying commands can be run directly:

```bash
go run ./cmd/gen/
go test ./...
```

---

## How to run the generator

```bash
make generate   # reads schemas/ and writes model/model.go
make test       # generates then runs go test ./...
make all        # same as above (default target)
make clean      # removes model/model.go
```

Running `make` with no arguments is equivalent to `make all`.

---

## Approach

### Iteration 1 — Schema parser

The first step was building a parser for the JSON Schema files before writing any code generation. The key challenge here is that the `type` field in JSON Schema can be either a plain string (`"string"`) or an array (`["string", "null"]`). A custom `SchemaType` unmarshaler normalises both forms into `[]string` so the rest of the generator never has to branch on which form was used.

Parsing was verified by printing a summary of every property across all three schemas — types, refs, enums, and oneOf counts — before writing a single line of Go output.

### Iteration 2 — Category (scalars, nullable fields, enums)

Category was the simplest schema to start with: no cross-file references, no nested objects, no oneOf. This iteration established the core helpers:

- `goName` converts JSON property names to exported Go identifiers, handling camelCase, hyphens, underscores, and Go initialisims (`id` → `ID`, `basic-science` → `BasicScience`).
- `goFieldType` maps a schema node to a Go type string.
- Nullable fields (`["string", "null"]`) map to pointer types (`*string`).
- String enums produce a named `string` type and a `const` block so invalid values are caught by the type system rather than at runtime.
- `go/format` is run over every generated file so the output is always gofmt-clean regardless of whitespace written during generation.

### Iteration 3 — Product (required fields, arrays, nested objects)

Product introduced three new cases:

- **Required fields**: the `required` array in the schema drives whether a struct tag gets `,omitempty`. Required fields are always present in JSON so omitting them from marshalled output would be wrong.
- **Arrays**: `tags: []string` and similar map to Go slice types. The items schema is inspected to determine the element type.
- **Nested inline objects**: `dimensions` has its own properties, so the generator recurses and produces a `ProductDimensions` sub-struct. Enums inside nested objects (`dimensions.unit`) produce their own named type (`ProductDimensionsUnit`).

At this point `generateCategory` was replaced with a general `generateObject` function that recurses into any depth of nesting. Category and Product now go through the same code path.

### Iteration 4 — Article ($ref, $defs, oneOf)

Article was the hardest schema. Three new concepts:

**Cross-file `$ref`** (`category: { "$ref": "Category.json" }`): the ref is resolved to a Go type name by stripping the `.json` suffix. No code generation is needed — the type already exists from a previous schema.

**Local `$defs`** (`$ref: "#/$defs/RichText"`): the three defs (`PlainText`, `RichText`, `RichTextNode`) are generated as top-level types before `Article` so they are in scope when the struct field references them. `RichTextNode` is self-referential (`children: []RichTextNode`) which works in Go without any special handling.

**`oneOf` union**: `Article.content` can be either `RichText` or `PlainText`. Go has no native sum type, so the generator produces a wrapper struct with one pointer field per variant and a custom `UnmarshalJSON` method. The method uses a discriminator strategy: it unmarshals only the `format` field first (a lightweight "probe"), then delegates to the correct concrete type based on its `const` value. The discriminator field is discovered automatically by scanning each variant's properties for a `const` value.

The generated `UnmarshalJSON` requires `encoding/json` and `fmt` in the output file. The generator detects whether any `oneOf` types were produced and conditionally adds the import block.

---

## Decisions and trade-offs

**Single output file (`model/model.go`)**: all generated types live in one file. The alternative — one file per schema — would be cleaner at scale but adds complexity to the generator with no benefit at this size.

**Alphabetical field ordering**: Go map iteration is non-deterministic, so property names are sorted before emission. This makes the output stable across runs (important for code review diffs) at the cost of not matching the original schema order.

**Pointer types for optional nested objects**: an optional nested object (not in `required`) is emitted as `*SubType` rather than `SubType`. This lets callers distinguish "field was absent" from "field was present but empty", which matters for JSON round-trips.

**No pointer for optional scalars**: optional scalar fields use `omitempty` without a pointer. A zero value (`""`, `0`, `false`) is omitted from marshalled output, which is acceptable for this schema set. Using pointers for every optional scalar would be more correct in theory but significantly noisier in practice.

**Enum as named `string` type**: enum values become typed constants (`CategoryKind`, `ArticleStatus`, etc.) rather than raw strings. This means callers can use the constants and the compiler rejects unknown values — a real correctness win with no runtime cost.

**`go/format` as the final step**: rather than carefully managing whitespace during generation, the generator writes loosely formatted source and runs `go/format` at the end. If the output contains a syntax error, `format.Source` returns an error and the file is not written — a useful early-warning gate.

---

## What would be added with more time

- **Validation**: the generator currently trusts the schemas. It could return errors for unsupported or malformed constructs rather than silently falling back to `any`.
- **Array of objects**: arrays whose `items` is an inline object (not a `$ref`) are not handled — they fall through to `[]any`. A recursive call into `generateObject` for the items schema would fix this.
- **`additionalProperties`**: schemas that allow arbitrary extra fields should map to `map[string]any` for the extra-properties portion.
- **Multiple output files**: splitting output by schema title would scale better and produce cleaner diffs.
