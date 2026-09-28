# Macro completion — implementation plan

**Branch:** `feat/macros` (base commit `a71f1bb`)
**Goal:** offer dbt macro names as completions, in the right places, and not in the wrong ones.
**In scope:** macros declared in this project's `macro-paths`, plus dbt's built-in context functions (`ref`, `source`, `config`, …).
**Out of scope for now:** macros from `dbt_packages/`, full UTF-16 position correctness, SQL-side completions.

---

## 1. What we're building, in one paragraph

When you're typing inside a Jinja tag in a dbt model — `{{ gene` — the server should
offer the macros you can actually call there. When you're typing inside
`ref('cust` it should offer model names and *not* macros, even though that's also
inside a Jinja tag. When you're inside a `{# comment #}` it should offer nothing.
Getting this right is almost entirely about answering two questions at the cursor:

1. **Is a macro name even legal at this spot?** (a grammar question)
2. **Which macros are visible from here?** (a scope question)

Everything in this plan exists to answer one of those two.

---

## 2. The mental model: how dbt files actually work

This section matters because it contradicts an assumption that's easy to make (and
that an earlier draft of this plan got wrong).

**dbt renders the Jinja template first, then hands the resulting text to the
database.** The template engine runs over the file as plain text. It has no idea
what SQL is. It does not know what a SQL string literal is. It does not know what
a SQL comment is.

The consequences are not intuitive:

```sql
-- this macro really does run, inside a SQL comment
-- {{ ref('customers') }}

-- this one really does run too, inside a SQL string
select '{{ generate_label() }}' as label
```

Both of those execute. The `ref` in the comment even creates a real dependency edge
in dbt's DAG. So **our scanner must be completely blind to SQL quoting and SQL
comments.** If we treat `'` as "entering a string, stop offering macros", we will be
wrong about how the language works.

The reverse case is also true and easy to miss — quotes *inside* a Jinja tag are
real and do hide the closing delimiter:

```sql
{{ some_macro("text containing }}", oth
                              ^^^^ this does NOT close the expression
```

The cursor after `oth` is still inside the Jinja expression, in the second argument.
So: **track quotes inside Jinja, ignore quotes outside it.** Exactly backwards from
what a SQL-first intuition suggests.

### Where a macro name is and isn't legal

Being inside a Jinja tag is necessary but not sufficient. `|` marks the cursor:

| Buffer | Offer macros? | Why |
|---|---|---|
| `{{ clean_\|` | yes | value position in an expression |
| `{{ outer_macro(\|` | yes | value position in an argument |
| `{% set result = clean_\|` | yes | value position on the right of `=` |
| `{% if check_\|` | yes | value position in a condition |
| `{% do clean_\|` | yes | value position |
| `{% call clean_\|` | yes | `call` takes a macro |
| `{% set clean_\|` | **no** | you're *naming* a new variable |
| `{% macro clean_\|` | **no** | you're *naming* a new macro |
| `{% for clean_\|` | **no** | you're *naming* a loop variable |
| `{% \|` | no | keyword position — offer `set`/`if`/`for`/`macro`/… |
| `{{ utilities.\|` | depends | resolve `utilities` first, then list its members |
| `{{ x is cust\|` | **no** | Jinja *test* namespace, a different list |
| `{{ x \| up\|` | **no** | Jinja *filter* namespace, a different list |
| `{{ ref('ord\|` | **no** | model names go here |
| `{{ "clean_\|" }}` | **no** | it's a string literal |
| `{# clean_\| #}` | **no** | Jinja comment |
| `{% raw %}{{ f\|` | **no** | raw block, nothing renders |
| `select \|` | no | plain SQL, no tag open |

The **negative** rows are the ones that matter. Offering macros everywhere inside
`{{ }}` is easy and produces a server that's noisy enough that people turn it off.

One more subtlety: macros can be passed around as values, not just called —
`{% set f = my_macro %}` is legal. So don't assume every completion must insert
`name(...)` with parentheses.

---

## 3. The pipeline

```
live buffer (didOpen / didChange)
      │
      ▼
snapshot: []rune + cursor as a rune offset
      │
      ▼
region scan  ──►  am I in Text / Expr / Stmt / Comment / Raw?
      │            (string-aware inside Jinja, blind to SQL quoting)
      ▼
role classification  ──►  Value? BindingName? Member? Filter? Test?
      │                    StringArg? plus receiver / callee / arg index
      ▼
visibility  ──►  locals (shadow everything) → project macros → builtins
      │
      ▼
candidates  ──►  trie prefix search
      │
      ▼
completion items with an explicit replace range
```

**Lexing tells you where you are. Scope tells you what's visible.**

Note what is *not* in that pipeline: a parser. Every row in the table above is
answerable by scanning regions and then walking backwards over tokens with a small
stack of open parentheses. A forgiving parser is a large amount of machinery whose
error-recovery heuristics are hard to test, and since we'd keep a token-based
fallback anyway, the fallback would be the path that actually runs — because during
completion the code is almost always half-finished. If we later want rename or
folding, those genuinely need a parser, and we build it then.

---

## 4. Where the codebase stands today

### 4.1 What already works

- Filesystem scanning and watching (`analysis/scanner.go`, `analysis/watcher.go`)
- Model index: filename → path, in a prefix trie
- Source index from YAML, with conflict detection
- `ref()` and `source()` completion, `ref()` go-to-definition
- Capability gating (`ServerCapabilitiesStatus`), including an unused `MacrosEnabled`

### 4.2 The macro index answers the wrong question

`dbt/macro.go:3`:

```go
type Macro struct {
	filePath string   // unexported — analysis/ can't read it back out
}
```

`analysis/macros.go:14`:

```go
s.DbtMacros.Put(
	strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtMacroExtension),
	macro,
)
```

The index maps **filename → an empty struct**. But dbt macro names have no
relationship to filenames. A single file routinely holds a dozen macros, and:

```sql
-- macros/utils.sql
{% macro grant_select(schema, role) %}
  grant select on all tables in schema {{ schema }} to {{ role }}
{% endmacro %}

{% macro cents_to_dollars(column_name, precision=2) %}
  round({{ column_name }} / 100, {{ precision }})
{% endmacro %}
```

is called as `grant_select(...)` and `cents_to_dollars(...)`. Never `utils(...)`.
So the current index is keyed on something a user will never type.

The trie itself is the *right* structure — prefix search is exactly what completion
needs. Only its contents are wrong.

Knock-on effect: `RemoveMacroFromIndex` (`analysis/macros.go:20`) deletes by
filename key. Once keys become macro names, removal needs a file → names reverse
map to know what to evict.

### 4.3 Completion never actually looks at the cursor

`analysis/completion.go:181-198`:

```go
line := getLine(doc.Data, params.Position.Line)
completionType, err := parseCompletionType(line)
```

and `analysis/completion.go:209`:

```go
func parseCompletionType(lineContent string) (string, error) {
	if sourceRe.MatchString(lineContent) { return "source", nil }
	if refRe.MatchString(lineContent)    { return "ref", nil }
	...
}
```

This regex-matches the **entire line** and ignores the cursor position completely.
Consequences:

```sql
{{ dbt_utils.star(from=ref('orders')) }}
             ^ cursor here
```

`refRe` matches somewhere on the line, so this classifies as `"ref"`. A macro branch
added alongside would never fire. Multi-line Jinja is invisible entirely:

```sql
{{ config(
     materialized='incremental',
     unique_key=
                ^ cursor — the line has no 'ref(' or 'source(', so: nothing
) }}
```

This is not extendable. It has to be replaced by the classifier — which also fixes
the ref/source misclassification as a side effect.

### 4.4 Three different position units are in play

| Where | Unit |
|---|---|
| LSP `Position.Character` | UTF-16 code units |
| `getOffset` (`analysis/rope.go:9`) | runes |
| `analysis/completion.go:42` | bytes |

That last one:

```go
Start: lsp.TextDocumentPosition{
	Line:      params.Position.Line,
	Character: params.Position.Character - len(modelRef),  // len() is BYTES
},
```

An ASCII-only project never notices. Anything else produces ranges off by the
number of multi-byte characters on the line.

### 4.5 `Documents` is accessed from multiple goroutines without a lock

`main.go:68` is a single reader loop. `server/dispatch.go:67`:

```go
if handler.stateful {
	s.handleMethod(state, method, contents, handler)   // inline, main goroutine
} else {
	go s.handleMethod(state, method, contents, handler) // spawned
}
```

`server/dispatch.go:52,57` mark completion and definition `stateful: false`. So:

- **main goroutine** writes: `analysis/document.go:24` `s.Documents[uri] = ...`
- **spawned goroutines** read: `analysis/completion.go:176`, `analysis/definition.go:24`

`State.Documents` (`analysis/state.go:32`) has no mutex. Three hazards:

1. **The map.** Concurrent map read + write is a Go *fatal error*
   (`fatal error: concurrent map read and map write`), not a recoverable panic.
   `recover()` cannot catch it. The process dies and the editor loses its server.

2. **The rope — a much wider window.** `applyUpdate` (`analysis/document.go:45`)
   mutates in place and calls `Rebalance()` every 200 edits:

   ```go
   if *doc.EditCount > 200 {
       doc.Data.Rebalance()
   }
   ```

   Meanwhile completion runs `getLine(doc.Data, ...)`, which walks the tree via
   `r.Each(...)`. A rebalance restructures the tree *while another goroutine is
   walking it* → garbage text, index panic, or a non-terminating walk. Locking only
   the map does not fix this; completion takes `*Document` out of the map and then
   dereferences `doc.Data`, which is being mutated regardless.

3. **`EditCount`.** `(*doc.EditCount)++` is a non-atomic read-modify-write. Minor.

**Why this hasn't bitten yet:** completion does one partial rope walk, and no
trigger characters are actually registered (see 4.6), so completion fires rarely.

**Why this plan changes that:** the classifier scans from offset 0 to the cursor
(a full walk), and D3 registers trigger characters for real. The interleaving is
structural — for one keystroke the client sends `didChange` then `completion` back
to back; didChange runs inline, completion is spawned; the *next* keystroke's
didChange then runs while the previous completion goroutine is still walking the
same rope. With trigger characters live that's the steady state of typing
`{{ ref(`, not an edge case.

### 4.6 Trigger characters never reach the client

`lsp/initialize.go:32`:

```go
CompletionProviderCapability struct {
	TriggerCharacters []rune `json:"triggerCharacters"`
}
```

`[]rune` is `[]int32`. It marshals to `[46]` — a JSON array of numbers — not
`["."]`. The client rejects or ignores it, so no trigger character is registered
today. Completion currently only fires on manual invoke or the client's own
word-prefix heuristics.

### 4.7 `CompletionItem` is missing what the design needs

`lsp/textdocument_completion.go:19`:

```go
type CompletionItem struct {
	Label         string             `json:"label"`
	Detail        string             `json:"detail"`
	Kind          int                `json:"kind"`
	Documentation string             `json:"documentation"`
	TextEdit      CompletionTextEdit `json:"textEdit"`
}
```

No `InsertText`, no `InsertTextFormat` (so no snippets), no `SortText` (so no
ranking), no `FilterText`, and no `omitempty` so empty strings always serialize.

### 4.8 Macro plumbing gaps

| Where | Problem |
|---|---|
| `analysis/watcher.go:47` `handleCreateEvent` | handles config + model files, no macro branch — new macro files never get indexed |
| `analysis/watcher.go:138` | passes `filepath.Base(event.Name)` into `isModelFile`/`isMacroFile`, but those test `strings.Contains(path, "macros")`. A basename can never contain the root dir, so the check always fails — for models too |
| `analysis/file_rules.go:29-41` | both predicates match `.sql`; `models/macros/x.sql` satisfies both |
| `analysis/scanner.go:136` `ScanRootPath` | scans models + config, not macros. `ScanWorkspaceRoots` calls it, so the watcher-restart path (`analysis/watcher.go:103`) silently drops the macro index |
| `analysis/sources.go:27` `resetProjectState` | resets `DbtModels`, not `DbtMacros` — stale macros survive project reconciliation |
| `analysis/completion.go:167` | never reads `IsMacrosEnabled()`; the gate exists and is set, nothing consumes it |
| `analysis/file_rules.go:10` | `SKIPPABLE_DIR = ["dbt_packages", "target"]` — package macros excluded by design |

---

## 5. Decisions already made

| Question | Decision | Consequence |
|---|---|---|
| `dbt_packages/` macros | **Out of scope** | No namespace resolution needed. Keep a `Package` field on the macro type anyway so the index shape survives adding it later. |
| dbt builtins (`ref`, `source`, …) | **In — hardcoded table** | Small static list in `dbt/`. High value: `{{ re` offering `ref()` is the single most common thing a user types. |
| UTF-16 vs runes | **Unify on runes, defer UTF-16** | Correct for the Basic Multilingual Plane. Still off for astral-plane characters (emoji). Document the gap; don't pretend it's fixed. |
| Prereq fixes | **Separate commits on this branch** | Matches the existing atomic commit style. |

---

## 6. The work

Each task below has a **Gist** (what and why, plain), **Do** (the actual
instructions), and **Done when**.

### Group A — prerequisites

These are invisible to users. Everything else sits on them.

---

#### A1 · `fix: guard document map against concurrent access`

**Gist.** Two goroutines touch the open-document store at the same time and nothing
stops them. Today it's a rare crash; after this plan it becomes a common one,
because completion will start doing much more work and firing much more often. See
§4.5 for the full argument.

**Do.**

1. Add to `State` in `analysis/state.go`:
   ```go
   DocumentsMu sync.RWMutex
   ```
2. `analysis/document.go` — take the write lock in `OpenDocument` and
   `UpdateDocument`. Note the lock must cover the *rope mutation*, not just the map
   lookup:
   ```go
   func (s *State) UpdateDocument(uri string, change lsp.TextDocumentContentChangeEvent, version int) {
       s.DocumentsMu.Lock()
       defer s.DocumentsMu.Unlock()

       doc, ok := s.Documents[uri]
       if !ok || doc == nil {
           s.Logger.Errorf("Update requested for unopened document: %s", uri)
           return
       }
       s.applyUpdate(doc, change, version)
   }
   ```
   Move the JSON-marshal logging above or below the critical section — don't hold
   the lock across it.
3. Leave completion and definition alone for now; A2 gives them a locked accessor.

> **Alternative, if you want the smaller diff:** flip completion and definition to
> `stateful: true` in `server/dispatch.go`. One word each. The race disappears
> entirely because everything serialises on the reader loop. Cost: a slow completion
> stalls didChange behind it, and request cancellation becomes impossible. At this
> project's scale completion should be sub-millisecond, so it's defensible. A2
> survives either way — you'd still want the snapshot for decoupling, it just
> wouldn't need a lock. **This is still an open decision (see §9).**

**Done when.** No unsynchronised access to `s.Documents` remains, and
`go test -race ./...` passes.

---

#### A2 · `feat: add rune-offset document snapshots`

**Gist.** Right now, anything that wants to read a document reaches into the rope
directly. That couples every consumer to the rope *and* forces them to hold a lock
for as long as they're reading. Instead, copy the text out once, atomically, and
hand back a plain `[]rune`.

This is the load-bearing decision of the whole plan. It means the new `jinja`
package takes a `[]rune` and knows nothing about ropes, locks, or `analysis` — so
it can be tested with string literals and no fixtures.

**Do.** New file `analysis/snapshot.go`:

```go
package analysis

import "github.com/fsorodrigues/dbt-ls/lsp"

// Snapshot is a detached copy of a document's text. It is safe to read
// without holding any lock; it will not observe edits made after it was taken.
type Snapshot struct {
	Text    []rune
	Version int
}

func (s *State) Snapshot(uri string) (Snapshot, bool) {
	s.DocumentsMu.RLock()
	defer s.DocumentsMu.RUnlock()

	doc, ok := s.Documents[uri]
	if !ok || doc == nil {
		return Snapshot{}, false
	}
	// Slice returns a fresh []rune, so the copy is detached from the rope.
	return Snapshot{
		Text:    doc.Data.Slice(0, doc.Data.Len()),
		Version: doc.Version,
	}, true
}

// Offset converts an LSP position to a rune offset into Text.
// Out-of-range positions clamp to the end of the document.
func (sn Snapshot) Offset(pos lsp.TextDocumentPosition) int {
	line, char := 0, 0
	for i, r := range sn.Text {
		if line == pos.Line && char == pos.Character {
			return i
		}
		if r == '\n' {
			line, char = line+1, 0
		} else {
			char++
		}
	}
	return len(sn.Text)
}

// Position converts a rune offset back to an LSP position.
func (sn Snapshot) Position(offset int) lsp.TextDocumentPosition {
	line, char := 0, 0
	for i := 0; i < offset && i < len(sn.Text); i++ {
		if sn.Text[i] == '\n' {
			line, char = line+1, 0
		} else {
			char++
		}
	}
	return lsp.TextDocumentPosition{Line: line, Character: char}
}

// Range builds an LSP range from a pair of rune offsets.
func (sn Snapshot) Range(start, end int) lsp.TextDocumentPositionRange {
	return lsp.TextDocumentPositionRange{
		Start: sn.Position(start),
		End:   sn.Position(end),
	}
}
```

**Known gap to document in a comment:** `Position.Character` is UTF-16 code units
per the LSP spec, and this counts runes. They agree for everything in the Basic
Multilingual Plane and disagree for astral-plane characters (emoji, some CJK
extensions). Accepted for now — see §5.

`getOffset` and `getLine` in `analysis/rope.go` stay for now; D1 removes their last
callers.

**Done when.** `Snapshot`, `Offset`, `Position`, `Range` exist with round-trip unit
tests (`Offset(Position(n)) == n` over a multi-line fixture including a non-ASCII
BMP character).

---

#### A3 · `refactor: expose dbt macro fields`

**Gist.** The macro type currently holds one unexported string that no other
package can read. It needs to describe a macro.

**Do.** Rewrite `dbt/macro.go`:

```go
package dbt

// Arg is a single declared macro parameter. Default is the literal source text
// of the default expression, or "" when the parameter has no default.
type Arg struct {
	Name    string
	Default string
}

// Macro is a callable Jinja macro declared in a dbt project.
type Macro struct {
	Name    string // the callable name, e.g. "cents_to_dollars"
	Package string // "" for this project; reserved for dbt_packages support
	File    string // absolute path to the declaring file
	Line    int    // 0-based line of the {% macro %} tag, for go-to-definition
	Args    []Arg
	Doc     string // adjacent doc comment, if any
	Builtin bool   // true for dbt context functions with no declaring file
}

func (m Macro) Signature() string { /* "name(a, b=2)" */ }
```

Update the one caller at `analysis/scanner.go:104`. It will be replaced wholesale
in C2, so a minimal fix is fine here.

**Done when.** `go build ./...` passes and `Macro` fields are readable from
`analysis`.

---

### Group B — the classifier (new `jinja/` package)

Lives in its own top-level package, consistent with `dbt/` already being separate.
It depends on nothing but the standard library — that's deliberate, it's what makes
it testable.

> **Naming warning:** `analysis/scanner.go` already exists and is a *filesystem*
> walker, not a lexer. Do not put the template scanner there.

---

#### B1 · `feat: add jinja region scanner`

**Gist.** Walk the file from the start to the cursor and keep track of which kind
of territory we're in: plain SQL, a `{{ }}` expression, a `{% %}` statement, a
`{# #}` comment, or a `{% raw %}` block. This is deliberately a scanner and not a
parser, because when someone is mid-completion the code is nearly always broken —
`{{ dbt_ut` has no closing brace — and a scanner handles that by construction.

**Do.** New file `jinja/scan.go`:

```go
package jinja

type Region uint8

const (
	RegionText    Region = iota // plain SQL, no tag open
	RegionExpr                  // inside {{ ... }}
	RegionStmt                  // inside {% ... %}
	RegionComment               // inside {# ... #}
	RegionRaw                   // inside {% raw %} ... {% endraw %}
)

// frame tracks one level of open parentheses inside a Jinja tag, so we know
// whose argument list the cursor sits in.
type frame struct {
	callee   string // identifier immediately preceding the '('
	argIndex int    // number of commas seen at this depth
}

// ScanState is the lexical situation at some offset.
type ScanState struct {
	Region Region
	Quote  rune    // the open quote inside a Jinja tag, or 0
	Stack  []frame // innermost call last
	OpenAt int     // offset of the '{{' or '{%' that opened the region
}

func ScanTo(src []rune, cursor int) ScanState
```

Scanning rules, by region:

**`RegionText`** — look only for `{{`, `{%`, `{#`. On `{%`, peek the first keyword:
`raw` enters `RegionRaw`, anything else enters `RegionStmt`. Skip a following `-`
(whitespace control, `{{-`) so the token walk doesn't mistake it for an operator.

> **Critical:** do **not** handle `'`, `"`, `--` or `/*` here. SQL quoting and SQL
> comments do not suppress Jinja (§2). Add a comment saying so, because it looks
> like an omission.

**`RegionExpr` / `RegionStmt`** — string-aware:

```go
func (sc *scanner) scanJinja() {
	c := sc.src[sc.pos]

	if sc.quote != 0 { // inside a string literal
		switch {
		case c == '\\':
			sc.pos += 2 // skip the escape and whatever it escapes
		case c == sc.quote:
			sc.quote = 0
			sc.pos++
		default:
			sc.pos++
		}
		return
	}

	switch {
	case c == '\'' || c == '"':
		sc.quote = c
		sc.pos++
	case sc.region == RegionExpr && sc.at("}}"):
		sc.region, sc.pos = RegionText, sc.pos+2
		sc.stack = sc.stack[:0]
	case sc.region == RegionStmt && sc.at("%}"):
		sc.region, sc.pos = RegionText, sc.pos+2
		sc.stack = sc.stack[:0]
	case c == '(':
		sc.stack = append(sc.stack, frame{callee: sc.identBefore(sc.pos)})
		sc.pos++
	case c == ')':
		if n := len(sc.stack); n > 0 {
			sc.stack = sc.stack[:n-1]
		}
		sc.pos++
	case c == ',':
		if n := len(sc.stack); n > 0 {
			sc.stack[n-1].argIndex++
		}
		sc.pos++
	default:
		sc.pos++
	}
}
```

Whitespace-control closers (`-}}`, `-%}`) need no special handling: at the `-` the
two-char match fails, we advance one, and then `}}` matches normally.

**`RegionComment`** — advance until `#}`.

**`RegionRaw`** — advance until the literal `{% endraw %}` (tolerate whitespace
variants). Nothing else exits. This is the one state that cannot be recovered by
looking near the cursor, which is a second reason the scan starts at offset 0.

**Done when.** `ScanTo` returns the correct `Region` for every row in B2.

---

#### B2 · `test: cover jinja region scanning`

**Gist.** Fixture strings with a caret marker, in a table. This harness is what
makes the feature maintainable — every bug in this area is an edge case, and edge
cases only stay fixed if they're in a table.

**Do.** `jinja/scan_test.go`. Use `|` as the caret marker and strip it to get the
offset. Match the repo's existing style — plain `t.Fatalf`, no assertion library
(none is in `go.mod`).

> **Gotcha:** the repo's `.gitignore` contains `*.sql`. Fixture files under
> `testdata/` would be silently untracked. **Use inline Go string literals**, or add
> a `!testdata/**` negation. Inline is better here anyway — the fixtures are one
> line each.

```go
func offsetOf(t *testing.T, marked string) ([]rune, int) {
	t.Helper()
	i := strings.Index(marked, "|")
	if i < 0 {
		t.Fatalf("fixture has no caret marker: %q", marked)
	}
	return []rune(marked[:i] + marked[i+1:]), len([]rune(marked[:i]))
}

func TestScanToRegion(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want Region
	}{
		{"plain sql",            "select |",                    RegionText},
		{"open expression",      "{{ clean_|",                  RegionExpr},
		{"closed expression",    "{{ x }} |",                   RegionText},
		{"open statement",       "{% set x = |",                RegionStmt},
		{"jinja comment",        "{# note |",                   RegionComment},
		{"raw block",            "{% raw %}{{ f|",              RegionRaw},
		{"raw closed",           "{% raw %}{{x}}{% endraw %}|", RegionText},
		{"brace inside string",  `{{ f("}}", g|`,               RegionExpr},
		{"escaped quote",        `{{ f('it\'s', g|`,            RegionExpr},
		{"whitespace control",   "{{- clean_|",                 RegionExpr},

		// SQL quoting and SQL comments are transparent to Jinja.
		{"jinja in sql string",  "select '{{ g|",               RegionExpr},
		{"jinja in sql comment", "-- {{ g|",                    RegionExpr},
	}
	// ...
}
```

Write the last three first. They're the cases the current line-regex gets wrong and
the ones most likely to regress.

Also add a fuzz target that calls `ScanTo` on truncated inputs and asserts it
doesn't panic or loop forever.

---

#### B3 · `feat: classify jinja completion context`

**Gist.** Knowing we're inside `{{ }}` isn't enough. Now figure out *what kind of
slot* the cursor is in — a value, a name being defined, a member after a dot, a
filter, a test, a string argument — and pull out the partial word the user has
typed plus the range to replace.

**Do.** New file `jinja/context.go`:

```go
package jinja

type Role uint8

const (
	RoleNone        Role = iota // offer nothing
	RoleValue                   // {{ HERE }} | {% set x = HERE %} | {% if HERE %}
	RoleStmtKeyword             // {% HERE %}
	RoleBindingName             // {% set HERE %} | {% macro HERE %} | {% for HERE in %}
	RoleMember                  // {{ adapter.HERE }}
	RoleFilter                  // {{ x | HERE }}
	RoleTest                    // {{ x is HERE }}
	RoleStringArg               // {{ ref('HERE') }}
	RoleKwargName               // {{ star(HERE=... ) }}
)

type Context struct {
	Region   Region
	Role     Role
	Receiver string // RoleMember: the identifier before the dot
	Callee   string // RoleStringArg / RoleKwargName: enclosing call
	ArgIndex int    // 0-based argument position within Callee
	Prefix   string // text from word start up to the cursor
	Start    int    // rune offset, start of the word under the cursor
	End      int    // rune offset, end of that word — may be past the cursor
}

func Classify(src []rune, cursor int) Context
```

Algorithm:

```go
func Classify(src []rune, cursor int) Context {
	st := ScanTo(src, cursor)

	// Dead zones: nothing renders, or no tag is open.
	switch st.Region {
	case RegionText, RegionComment, RegionRaw:
		return Context{Region: st.Region, Role: RoleNone}
	}

	ctx := Context{Region: st.Region}
	ctx.Start, ctx.End, ctx.Prefix = wordAround(src, cursor)

	// Inside a string literal in a Jinja tag: this is an argument value,
	// e.g. ref('ord|'). The candidate set comes from the callee, not macros.
	if st.Quote != 0 {
		ctx.Role = RoleStringArg
		if n := len(st.Stack); n > 0 {
			ctx.Callee = st.Stack[n-1].callee
			ctx.ArgIndex = st.Stack[n-1].argIndex
		}
		return ctx
	}

	// Walk backwards over the tokens before the word.
	switch prev := prevSignificant(src, ctx.Start); {
	case prev.is("."):
		ctx.Role, ctx.Receiver = RoleMember, identBefore(src, prev.start)
	case prev.is("|"):
		ctx.Role = RoleFilter
	case prev.is("is"), prev.is("not") && prevIs(src, prev.start, "is"):
		ctx.Role = RoleTest
	case st.Region == RegionStmt && isFirstWordInTag(src, st.OpenAt, ctx.Start):
		ctx.Role = RoleStmtKeyword
	case st.Region == RegionStmt && isBindingSlot(src, st.OpenAt, ctx.Start):
		// {% set NAME %}, {% macro NAME %}, {% for NAME in ... %}
		ctx.Role = RoleBindingName
	case nextSignificantIs(src, ctx.End, "="):
		ctx.Role = RoleKwargName
	default:
		ctx.Role = RoleValue
	}
	return ctx
}
```

`isBindingSlot` is the one worth spelling out. The cursor is a binding name when
the tag's keyword is `set`/`macro` and the cursor is in the *first* word after it
with no `=` yet seen, or when the keyword is `for` and no `in` has been seen. Note
the asymmetry: `{% set clean_| %}` is a binding but `{% set x = clean_| %}` is a
value — the `=` flips it.

`wordAround` walks both directions, which is what makes mid-word completion work:

```go
func wordAround(src []rune, cursor int) (start, end int, prefix string) {
	start = cursor
	for start > 0 && isIdentRune(src[start-1]) {
		start--
	}
	end = cursor
	for end < len(src) && isIdentRune(src[end]) {
		end++
	}
	return start, end, string(src[start:cursor])
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
```

`Prefix` is only the text *before* the cursor (what we filter on). `[Start, End)` is
the whole word (what we replace). When the cursor is mid-word those differ, and
that difference is what LSP's insert-vs-replace ranges exist for.

**Done when.** Every row in §2's table produces the right `Role`.

---

#### B4 · `test: cover jinja context classification`

**Do.** `jinja/context_test.go`, same caret-marker harness:

```go
cases := []struct {
	src      string
	role     Role
	prefix   string
	receiver string
	callee   string
}{
	{"{{ dbt_ut|",            RoleValue,       "dbt_ut", "", ""},
	{"{{ outer(inner(a, b|",  RoleValue,       "b",      "", ""},
	{"{{ ref('my_|')",        RoleStringArg,   "my_",    "", "ref"},
	{"{{ source('a', 'b|')",  RoleStringArg,   "b",      "", "source"}, // ArgIndex 1
	{"{% set x = cl|",        RoleValue,       "cl",     "", ""},
	{"{% set cl|",            RoleBindingName, "cl",     "", ""},
	{"{% macro cl|",          RoleBindingName, "cl",     "", ""},
	{"{% for ro|",            RoleBindingName, "ro",     "", ""},
	{"{% if check_|",         RoleValue,       "check_", "", ""},
	{"{% call cl|",           RoleValue,       "cl",     "", ""},
	{"{% en|",                RoleStmtKeyword, "en",     "", ""},
	{"{{ adapter.dis|",       RoleMember,      "dis",    "adapter", ""},
	{"{{ x is cu|",           RoleTest,        "cu",     "", ""},
	{"{{ y | up|",            RoleFilter,      "up",     "", ""},
	{"{{ star(fr|=1)",        RoleKwargName,   "fr",     "", "star"},
	{"{# cl|",                RoleNone,        "",       "", ""},
	{"{% raw %}{{ f|",        RoleNone,        "",       "", ""},
	{"select |",              RoleNone,        "",       "", ""},
	{`{{ f("}}", g|`,         RoleValue,       "g",      "", "f"},
	{"select '{{ g|",         RoleValue,       "g",      "", ""},
	{"-- {{ g|",              RoleValue,       "g",      "", ""},
}
```

Add one mid-word case asserting `Start < cursor < End`:
`{{ cle|an_name }}` → `Prefix == "cle"`, `End` past `an_name`.

---

### Group C — the index

---

#### C1 · `feat: parse macro definitions from source`

**Gist.** Read a `.sql` file in the macros directory and pull out every macro it
declares — name, parameters, the line it starts on, and any doc comment sitting
above it.

**Do.** New file `jinja/definitions.go`, reusing the region scanner:

```go
package jinja

type DefKind uint8

const (
	DefMacro DefKind = iota
	DefMaterialization
	DefTest
)

type Param struct {
	Name    string
	Default string // literal source of the default expression, "" if none
}

type Definition struct {
	Kind   DefKind
	Name   string
	Params []Param
	Line   int    // 0-based, of the opening tag
	Doc    string // adjacent {# ... #} comment immediately above, if any
}

// Definitions extracts every macro-like declaration in src.
func Definitions(src []rune) []Definition
```

Implementation: scan the whole file. Every time a `RegionStmt` opens, read the first
keyword. On `macro` / `materialization` / `test`, read the name, then the
parenthesised parameter list. Parameter defaults are captured as raw source text —
we're not evaluating Jinja, just recording what's there.

Parse this correctly:

```sql
{#- Converts an integer cents column to dollars. -#}
{% macro cents_to_dollars(column_name, precision=2) -%}
    round({{ column_name }} / 100, {{ precision }})
{%- endmacro %}
```

→ `Definition{Kind: DefMacro, Name: "cents_to_dollars", Line: 1,
Params: [{column_name, ""}, {precision, "2"}],
Doc: "Converts an integer cents column to dollars."}`

**Done when.** A multi-macro fixture with defaults, whitespace control, and doc
comments round-trips.

---

#### C2 · `feat: key macro index by macro name`

**Gist.** Replace the filename-keyed index with a name-keyed one, so looking up
`cents_to` actually finds something. Because one file declares many macros, we also
need to remember which names came from which file, or we can't clean up when a file
is deleted.

**Do.**

1. `analysis/state.go` — change the trie's value type and add the reverse map:
   ```go
   DbtMacrosMu   sync.Mutex
   DbtMacros     *trie.Trie[[]dbt.Macro]  // macro name -> declarations
   DbtMacroFiles map[string][]string      // file path -> names it declares
   ```
   The slice value handles a name being declared in more than one place — today
   that's a user error worth surfacing later; with packages it becomes normal.

2. Rewrite `analysis/macros.go`:
   ```go
   // AddMacroFile parses file and indexes every macro it declares,
   // replacing any previous declarations from the same file.
   func (s *State) AddMacroFile(path string) {
       data, err := os.ReadFile(path)
       if err != nil {
           s.Logger.Errorf("Error reading macro file %s: %s", path, err)
           return
       }
       defs := jinja.Definitions([]rune(string(data)))

       s.DbtMacrosMu.Lock()
       defer s.DbtMacrosMu.Unlock()

       s.removeMacroFileLocked(path) // idempotent re-index

       names := make([]string, 0, len(defs))
       for _, d := range defs {
           m := dbt.Macro{
               Name: d.Name, File: path, Line: d.Line,
               Args: toArgs(d.Params), Doc: d.Doc,
           }
           key := strings.ToLower(d.Name)
           existing, _ := s.DbtMacros.Get(key)
           s.DbtMacros.Put(key, append(existing, m))
           names = append(names, key)
       }
       s.DbtMacroFiles[path] = names
   }

   func (s *State) RemoveMacroFile(path string) {
       s.DbtMacrosMu.Lock()
       defer s.DbtMacrosMu.Unlock()
       s.removeMacroFileLocked(path)
   }

   // removeMacroFileLocked drops only the declarations that came from path,
   // leaving same-named macros declared elsewhere intact.
   func (s *State) removeMacroFileLocked(path string) {
       for _, key := range s.DbtMacroFiles[path] {
           decls, ok := s.DbtMacros.Get(key)
           if !ok {
               continue
           }
           kept := decls[:0]
           for _, m := range decls {
               if m.File != path {
                   kept = append(kept, m)
               }
           }
           if len(kept) == 0 {
               s.DbtMacros.Remove(key)
           } else {
               s.DbtMacros.Put(key, kept)
           }
       }
       delete(s.DbtMacroFiles, path)
   }
   ```

3. `analysis/scanner.go:103-106` — replace the `dbt.NewMacro(file)` /
   `AddNewMacroToIndex` pair with `s.AddMacroFile(file)`.

4. Initialise `DbtMacroFiles` in `NewState`.

**Done when.** A file declaring three macros produces three trie entries; deleting
that file removes exactly those three and leaves same-named macros from other files
alone.

---

#### C3 · `feat: add dbt builtin context functions`

**Gist.** `ref`, `source`, `config` and friends aren't declared in any file — dbt
injects them. They're also the things people type most. Ship a static list.

**Do.** New file `dbt/builtins.go`:

```go
package dbt

// Builtins are the dbt Jinja context members. They have no declaring file,
// so Builtin is true and File/Line are empty.
//
// Source: https://docs.getdbt.com/reference/dbt-jinja-functions
var Builtins = []Macro{
	{Name: "ref", Builtin: true, Args: []Arg{{Name: "model_name"}},
		Doc: "Reference another model in the project."},
	{Name: "source", Builtin: true, Args: []Arg{{Name: "source_name"}, {Name: "table_name"}},
		Doc: "Reference a table declared in a sources YAML file."},
	{Name: "config", Builtin: true,
		Doc: "Set model configuration from within the model."},
	{Name: "var", Builtin: true, Args: []Arg{{Name: "name"}, {Name: "default"}}},
	{Name: "env_var", Builtin: true, Args: []Arg{{Name: "name"}, {Name: "default"}}},
	{Name: "is_incremental", Builtin: true},
	{Name: "this", Builtin: true},
	{Name: "target", Builtin: true},
	{Name: "log", Builtin: true, Args: []Arg{{Name: "msg"}, {Name: "info", Default: "False"}}},
	{Name: "run_query", Builtin: true, Args: []Arg{{Name: "sql"}}},
	{Name: "statement", Builtin: true},
	{Name: "return", Builtin: true, Args: []Arg{{Name: "value"}}},
	{Name: "adapter", Builtin: true},
	{Name: "exceptions", Builtin: true},
	{Name: "modules", Builtin: true},
	{Name: "graph", Builtin: true},
	{Name: "invocation_id", Builtin: true},
	{Name: "run_started_at", Builtin: true},
	{Name: "dbt_version", Builtin: true},
	{Name: "flags", Builtin: true},
	{Name: "print", Builtin: true, Args: []Arg{{Name: "msg"}}},
	{Name: "tojson", Builtin: true}, {Name: "fromjson", Builtin: true},
	{Name: "toyaml", Builtin: true}, {Name: "fromyaml", Builtin: true},
	{Name: "load_result", Builtin: true, Args: []Arg{{Name: "name"}}},
	{Name: "selected_resources", Builtin: true},
	{Name: "builtins", Builtin: true},
}

func BuiltinsWithPrefix(prefix string) []Macro { /* case-insensitive */ }
```

Keep them out of the trie. The trie is the project index and gets reset on project
reconciliation; builtins are constant. Merge the two lists at completion time.

**Done when.** `BuiltinsWithPrefix("re")` returns `ref` and `return`.

---

#### C4 · `test: cover macro definition parsing and indexing`

Table tests for `jinja.Definitions`, plus index tests for add / re-add / remove with
name collisions across files.

---

### Group D — wiring it up

---

#### D1 · `refactor: route completion through jinja context`

**Gist.** Rip out the whole-line regex and drive completion from the classifier
instead. Ref and source completion keep working — they just stop guessing. This
alone fixes the `{{ dbt_utils.star(from=ref('x')) }}` misclassification.

**Do.** Rewrite `TextDocumentCodeCompletion` in `analysis/completion.go`:

```go
func (s *State) TextDocumentCodeCompletion(id int, params lsp.CompletionParams) lsp.CompletionResponse {
	response := NewCompletionResponse(id)
	if !s.IsServerActive() {
		return *response
	}

	snap, ok := s.Snapshot(params.TextDocument.URI)
	if !ok {
		s.Logger.Errorf("Completion requested for unopened document: %s", params.TextDocument.URI)
		return *response
	}

	cursor := snap.Offset(params.Position)
	ctx := jinja.Classify(snap.Text, cursor)
	s.Logger.Tracef("Completion context: %+v", ctx)

	switch ctx.Role {
	case jinja.RoleStringArg:
		switch ctx.Callee {
		case "ref":
			if s.IsRefCompletionEnabled() {
				s.createRefResponse(snap, ctx, response)
			}
		case "source":
			if s.IsSourceCompletionEnabled() {
				s.createSourceResponse(snap, ctx, response)
			}
		}
	case jinja.RoleValue:
		if s.IsMacrosEnabled() {
			s.createMacroResponse(snap, ctx, response) // D2
		}
	}

	return *response
}
```

Then:

- **Delete** `parseCompletionType`, `sourceRe`, `refRe`,
  `extractModelRefUnderCursor`, `extractSourceContextUnderCursor`, and
  `SourceCompletionContext`.
- Rework `createRefResponse` / `createSourceResponse` to take `(Snapshot, jinja.Context)`.
  Their trie/config lookup bodies survive; only the "figure out what the user typed"
  part goes. Source name vs. table name now comes from `ctx.ArgIndex` (0 vs 1)
  instead of two regexes plus two partial-match fallbacks.
- **Ranges** now come from the context, killing the byte/UTF-16 mixing:
  ```go
  TextEdit: lsp.CompletionTextEdit{
      Range:   snap.Range(ctx.Start, ctx.End),
      NewText: modKey,
  },
  ```
- `analysis/definition.go` gets the same treatment — snapshot + `Classify`, look for
  `RoleStringArg` with `Callee == "ref"`.
- `getLine` in `analysis/rope.go` should now be unused. Delete it. `getOffset` is
  still used by `applyUpdate`; leave it.

**Done when.** Existing ref/source behaviour is unchanged, plus
`{{ dbt_utils.star(from=ref('ord|')) }}` offers models and
`{{ config(materialized='incr|') }}` offers nothing.

---

#### D2 · `feat: complete macro names in jinja expressions`

**Gist.** The actual feature. In a value position, offer project macros and dbt
builtins matching what's been typed.

**Do.** New `createMacroResponse` in `analysis/completion.go`:

```go
func (s *State) createMacroResponse(snap Snapshot, ctx jinja.Context, response *lsp.CompletionResponse) {
	prefix := strings.ToLower(ctx.Prefix)

	s.DbtMacrosMu.Lock()
	var found []dbt.Macro
	for _, key := range s.DbtMacros.KeysWithPrefix(prefix) {
		if decls, ok := s.DbtMacros.Get(key); ok {
			found = append(found, decls...)
		}
	}
	s.DbtMacrosMu.Unlock()

	found = append(found, dbt.BuiltinsWithPrefix(prefix)...)

	for _, m := range found {
		response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
			Label:         m.Name,
			Kind:          lsp.CompletionItemKindFunction,
			Detail:        m.Signature(),
			Documentation: m.Doc,
			SortText:      macroSortText(m),
			TextEdit: lsp.CompletionTextEdit{
				Range:   snap.Range(ctx.Start, ctx.End),
				NewText: m.Name,
			},
		})
	}
}
```

**Ranking** via `SortText` — LSP sorts lexicographically on it, so prefix each tier
with a digit:

```go
func macroSortText(m dbt.Macro) string {
	switch {
	case strings.HasPrefix(m.Name, "_"):   return "3" + m.Name // private convention
	case strings.Contains(m.Name, "__"):   return "3" + m.Name // adapter dispatch impl
	case m.Builtin:                        return "1" + m.Name
	default:                               return "0" + m.Name // this project's macros
	}
}
```

**Adapter dispatch.** dbt projects contain `default__my_macro`,
`snowflake__my_macro` etc. — implementations, not call targets. Users call
`my_macro`. Ranking them to the bottom is the cheap move for this pass; collapsing
them into the dispatched name is better but needs `adapter.dispatch` awareness, so
leave it.

**Insertion:** insert the bare name, no parentheses. Macros can be passed as values
(`{% set f = my_macro %}`), and there's no snippet support wired up yet. Revisit
once `InsertTextFormat` is in (D3) — but note that snippets need a client capability
check, which means parsing `initialize` params we currently discard.

**Done when.** Typing `{{ ce` in a model offers `cents_to_dollars` from
`macros/utils.sql`, and `{{ re` offers `ref` and `return`.

---

#### D3 · `feat: extend completion item and trigger characters`

**Gist.** Two protocol-level fixes. The trigger-character list is currently
malformed on the wire so the editor ignores it, and `CompletionItem` is missing the
fields needed for ranking and snippets.

**Do.**

`lsp/initialize.go` — `[]rune` marshals to `[46]`, not `["."]`:

```go
CompletionProviderCapability struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}
```
```go
CompletionProvider: CompletionProviderCapability{
	TriggerCharacters: []string{"{", "%", ".", "(", "'", "\"", "|"},
},
```

`lsp/textdocument_completion.go`:

```go
const (
	CompletionItemKindFunction  = 3
	CompletionItemKindVariable  = 6
	CompletionItemKindReference = 18
)

const (
	InsertTextFormatPlainText = 1
	InsertTextFormatSnippet   = 2
)

type CompletionItem struct {
	Label            string             `json:"label"`
	Kind             int                `json:"kind"`
	Detail           string             `json:"detail,omitempty"`
	Documentation    string             `json:"documentation,omitempty"`
	SortText         string             `json:"sortText,omitempty"`
	FilterText       string             `json:"filterText,omitempty"`
	InsertText       string             `json:"insertText,omitempty"`
	InsertTextFormat int                `json:"insertTextFormat,omitempty"`
	TextEdit         CompletionTextEdit `json:"textEdit"`
}
```

Also reconsider `NewCompletionResponse` hardcoding `IsIncomplete: true`
(`analysis/completion.go:161`). `true` tells the client to re-query on every
keystroke rather than filter locally. That's correct while the prefix is short and
wasteful once it's specific. Set it from whether results were actually truncated.

**Done when.** The `initialize` response contains
`"triggerCharacters":["{","%",".","(","'","\"","|"]` and typing `{` in a model
triggers a completion request.

---

#### D4 · `test: cover macro completion routing`

End-to-end through `TextDocumentCodeCompletion`: build a `State`, open a document
with known text, request completion at a marked offset, assert on the returned
labels. Covers the wiring the `jinja` unit tests can't.

---

### Group E — plumbing fixes

Independent of everything above. Can land at any point.

---

#### E1 · `fix: classify watched files by full path`

**Gist.** The file-type predicates check whether the path sits under a configured
root, but the watcher hands them a bare filename. The check can therefore never
succeed — for models as well as macros.

**Do.** `analysis/watcher.go` — pass `event.Name` (the full path) to
`isModelFile`/`isMacroFile`/`isConfigFile`, not `base`. Keep `base` only for
`isSkippableTempArtifact`. Same fix at `handleCreateEvent:73-82`.

While in there, tighten `analysis/file_rules.go`. `strings.Contains(path, sub)` is
too loose — a model at `models/staging/macros_helper.sql` matches the *macro*
predicate. Match against the resolved root prefix instead:

```go
func (s *State) isUnderRoots(path string, roots []string) bool {
	abs := filepath.Clean(path)
	for _, root := range roots {
		full := filepath.Clean(filepath.Join(s.ProjectRoot, root))
		if rel, err := filepath.Rel(full, abs); err == nil &&
			!strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}
```

**Done when.** A test creating `models/x.sql` and `macros/y.sql` classifies each
correctly, and `models/staging/macros_helper.sql` is a model only.

---

#### E2 · `fix: index macro files on create and write`

**Gist.** Creating a new macro file does nothing, and saving one does nothing. Only
deletes and renames are handled.

**Do.**

- `handleCreateEvent` (`analysis/watcher.go:47`) — add a macro branch calling
  `s.AddMacroFile(path)`.
- The write handler (`analysis/watcher.go:160-164`) currently `continue`s on macro
  files. Call `s.AddMacroFile(event.Name)` — C2 made it idempotent.
- Consider reusing the content-hash dedupe from `ProcessNewConfigYaml`
  (`analysis/sources.go:233`). Neovim's atomic save fires several fsnotify events
  per save; without dedupe every save reparses the file three or four times.

**Done when.** Creating and then editing a macro file updates the index without a
restart.

---

#### E3 · `fix: scan and reset macro index with project lifecycle`

**Gist.** Two lifecycle paths forget macros exist.

**Do.**

- `ScanRootPath` (`analysis/scanner.go:136`) — add `ScanMacroRoots`. Without it
  `ScanWorkspaceRoots` → watcher restart (`analysis/watcher.go:103`) silently drops
  every macro.
  ```go
  func (s *State) ScanRootPath(rootPath string) error {
      if err := s.ScanModelRoots(rootPath); err != nil { return err }
      if err := s.ScanMacroRoots(rootPath); err != nil { return err }
      return s.ScanConfigRoot(rootPath)
  }
  ```
- `resetProjectState` (`analysis/sources.go:27`) resets `DbtModels` but not
  `DbtMacros`. Add:
  ```go
  s.DbtMacrosMu.Lock()
  s.DbtMacros = trie.New[[]dbt.Macro]()
  s.DbtMacroFiles = map[string][]string{}
  s.DbtMacrosMu.Unlock()
  ```

**Done when.** Editing `dbt_project.yml` to change `macro-paths` reindexes cleanly
with no stale entries.

---

### Group F — optional follow-on

#### F1 · `feat: go to definition for macros`

Nearly free after C2 — the index carries `File` and `Line`. Add a `RoleValue` branch
to `TextDocumentGoToDefinition` that looks up the name and returns
`Location{URI: file://..., Range: line}`.

Worth noting: `analysis/definition.go:42` hardcodes line 0 even for models. Once
macros need real line numbers, models can use them too.

---

## 7. Dependency order

```
A1 ──► A2 ──┬──► D1 ──► D2 ──► D4
            │           ▲
A3 ──► C2 ──┴───────────┤
       ▲                │
B1 ──► B3 ──► B4        │
 │      ▲               │
 │      └── (B1 also feeds C1)
 └───► B2               │
                        │
C1 ──► C2               │
C3 ─────────────────────┘

D3  independent of the rest of D, but D2's SortText is inert without it
E1, E2, E3  free-floating (E2 depends on C2 for AddMacroFile)
```

Suggested landing order: **A → B → C → D → E**.

Alternative if you want something visible sooner: **E first**. It makes macro
indexing actually work end to end, which gives you a real index to inspect while the
classifier is being built. E2 needs `AddMacroFile` from C2, so E-first means
E1 → E3 → C1 → C2 → E2.

---

## 8. Risks and things that will surprise you

**`.gitignore` contains `*.sql`.** Any `testdata/*.sql` fixture is silently
untracked — tests pass locally, fail in CI or on a fresh clone. Use inline string
literals, or add a `!testdata/**` negation.

**The classifier is O(document) per keystroke.** Scanning from offset 0 plus the
snapshot copy. A 500-line model is roughly 20 KB of runes — negligible. Caching
scanner state per line start is a real optimisation but premature; noting it so it
isn't mistaken for an oversight.

**`{% raw %}` cannot be recovered locally.** Determining you're inside one requires
having seen the opener. This is a second reason the scan starts at offset 0 rather
than backtracking from the cursor.

**Model and macro files are both `.sql`.** `DbtModelExtension` and
`DbtMacroExtension` are both `".sql"` (`analysis/state.go:263,265`). The only
discriminator is the directory. E1 makes that check correct, but the ambiguity is
structural — a dbt project can legally configure overlapping paths.

**`IsIncomplete: true` is currently unconditional.** With trigger characters live
(D3), this means the client re-queries on every single keystroke instead of
filtering the list it already has. Worth fixing in the same pass.

**Snapshot allocates on every completion.** `Slice(0, Len())` copies the whole
document. Acceptable for dbt models; if it ever shows up in a profile, slice only up
to the cursor — the classifier never reads past it except in `wordAround`, which
needs a small lookahead.

**Client capability negotiation is discarded.** `InitializeRequestParams`
(`lsp/initialize.go:16`) only captures `ClientInfo` and `WorkspaceFolders`. Snippet
support, insert-replace support, and `positionEncoding` all live in the client
capabilities we throw away. Anything conditional on those needs that struct widened
first.

---

## 9. Open questions

1. **A1 approach.** Mutex + snapshot, or flip completion/definition to
   `stateful: true` and serialise everything on the reader loop? See the note under
   A1. Recommendation: mutex, because the snapshot boundary is wanted for A2
   regardless — but the serialising option is genuinely defensible at this scale.

2. **Does `{% docs %}` matter?** C1 reads doc comments adjacent to the macro. dbt
   also has `{% docs name %}` blocks in `.md` files referenced via `{{ doc('name') }}`.
   Out of scope here, but it's the natural source for richer hover text later.

3. **Should `RoleMember` do anything in this pass?** With packages out of scope, the
   only receivers are `adapter`, `this`, `target`, `loop`, `modules`, `exceptions`,
   `graph`. Each would need its own member list. Cheapest correct behaviour for now
   is to return nothing for `RoleMember` — which is at least not *wrong*, unlike
   offering the full macro list after a dot.
