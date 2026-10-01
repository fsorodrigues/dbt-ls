# dbt language server

A (WIP) dbt language server implemented in Go.

## Context

dbt's language server is a proprietary part of the dbt's new fusion
engine, and they say they have no plans to make it available outside of the
official VSCode 🤮 extension. So I decided to build one from the ground up
to have access to some smart functionalities for dbt projects I work on.

This is a personal project, built for my own needs around my personal setup
(neovim, btw). Use it at your own peril.

## Capabilities

At this stage, it offers minimal functionality (see above about personal
project). Check the [Current limitations](#current-limitations) below for what
it deliberately doesn't do.

- [dbt language server](#dbt-language-server)
  - [Context](#context)
  - [Capabilities](#capabilities)
    - [Model name completion](#model-name-completion)
    - [Source name completion](#source-name-completion)
    - [Source table completion](#source-table-completion)
    - [Macro completion](#macro-completion)
    - [Jump to model from ref](#jump-to-model-from-ref)
    - [Jump to macro definition](#jump-to-macro-definition)
    - [Current limitations](#current-limitations)
  - [Requirements](#requirements)
  - [Installation](#installation)
    - [Compiling from source](#compiling-from-source)
  - [Editor Configuration](#editor-configuration)
    - [Neovim](#neovim)
      - [Triggering definition jumps](#triggering-definition-jumps)

### Model name completion

Autocompletes dbt model names inside `ref('...')` macros. Models come from the
`.sql` files in the `model-paths` configured in `dbt_project.yml`; when omitted,
`model-paths` defaults to `models`, matching dbt. Typing `ref('my_mod')`
suggests available models matching `my_mod`.

![Model name completion suggestion when on ref tag](./docs/images/ref.gif)

### Source name completion

Autocompletes dbt source names in the first argument of `source()`. Sources come
from your dbt project's YAML configuration files. Typing `source('my_src')`
suggests available sources matching `my_src`.

![Source name completion suggestion when on source tag](./docs/images/source.gif)

### Source table completion

Autocompletes table names in the second argument of `source()`. Tables come from
the selected source's YAML definition. After entering a valid source name,
typing `source('my_source', 'my_ta')` suggests matching tables such as
`my_table` and `my_table_v2`.

![Source table completion suggestion](./docs/images/source-table.gif)

### Macro completion

Suggests dbt macro names while you type Jinja. The LS reads the `.sql` files in
your project's `macro-paths` (defaults to `macros` when omitted, matching dbt)
and indexes every `{% macro %}` it finds, so typing a name inside a Jinja tag
(e.g. `{{ grant_sel`) suggests the macros matching the input.

Project macros show their full signature (e.g. `grant_select(schema, role)`) and
insert the call with the argument names already in place, so accepting that item
writes `grant_select(schema, role)` for you. Each one also carries the file it
was declared in, shown alongside the signature in the completion detail.

dbt's built-in Jinja context members (`ref`, `source`, `config`, `var`,
`env_var`, `is_incremental`, `this`, ...) are folded into the same list, so
`{{ re` offers `ref` alongside your own macros. Built-ins insert just the bare
name, since some of them are values rather than callables.

Your own macros rank first. Names prefixed with `_` or containing `__` (like
`default__grant_select` or `_log_helper`) sort last, keeping adapter dispatch
implementations and private helpers out of the way without hiding them. A
project macro that shadows a built-in wins over it.

![Macro completion with argument placeholders](./docs/images/macro.gif)

Suggestions only appear where a macro name is meaningful: inside `{{ ... }}` and
value positions in `{% ... %}`. They stay out of string arguments such as
`ref('ord')`, keyword positions, Jinja comments, and `{% raw %}` blocks.

### Jump to model from ref

Enables "Go to Definition" functionality for dbt models. Triggering your
editor's definition jump command while the cursor is on a model name inside a
`ref('...')` macro will open the corresponding model's source file.

See [Triggering definition jumps](#triggering-definition-jumps) for how to wire
that up in your editor.

![Go to definition of model under cursor](./docs/images/go-to-definition.gif)

### Jump to macro definition

Enables "Go to Definition" for macros declared in your project. Triggering your
editor's definition jump command while the cursor is on a macro name opens the
file that declares it, positioned at the `{% macro %}` line.

Built-ins are skipped here: they have no declaring file to jump to.

![Go to definition of macro under cursor](./docs/images/macro-go-to-definition.gif)


See [Triggering definition jumps](#triggering-definition-jumps) for how to wire
that up in your editor.

### Current limitations

- Macros installed from packages under `dbt_packages/` are not indexed, so
  completion and definition won't see macros from `dbt_utils` and friends.
- Macro completion only applies inside Jinja (`{{ ... }}` and value positions in
  `{% ... %}`). Filters (`{{ x | uppe }}`) and tests (`{{ x is cust }}`) belong
  to Jinja's own namespaces and are not completed.
- No SQL syntax diagnostics. There are solid SQL-specific LS and linters out
  there for that, and they compose fine with this project.

## Requirements

- **Go**: >= 1.25
- **Neovim**: >= 0.8

## Installation

### Compiling from source

The recommended way:

```bash
go install github.com/fsorodrigues/dbt-ls@latest
```

This installs the `dbt-ls` binary into `$(go env GOPATH)/bin` (or `$GOBIN` if
set). Make sure that directory is on your `PATH`.

You can also build from a local clone:

```bash
git clone https://github.com/fsorodrigues/dbt-ls
cd dbt-ls
go install .

# or
go build -o /path/to/your/dbt-ls .
```

## Editor Configuration

### Neovim

If you're using Neovim >= 0.11, the ls can be easily configured by adding the
following to your `init.lua` or a dedicated configuration file. (this uses the
native `vim.lsp.config` and `vim.lsp.enable` APIs). 

```lua
vim.lsp.config('dbt-ls', {
  cmd = {
    "/path/to/your/dbt-ls", -- i.e. /usr/local/bin/dbt-ls
    "--log-dir",
    "/path/to/your/dbt-ls_logs", -- optional; defaults to ${XDG_STATE_HOME:-~/.local/state}/dbt-ls/logs/
    "--log-level",
    "debug", -- use "trace" for the most verbose logs
  },
  filetypes = { "sql" },
  root_markers = { "dbt_project.yml", "dbt_project.yaml", ".git" },
})

vim.lsp.enable({
    -- other ls ...
    'dbt-ls',
    -- ... other ls
})
```

If you're on earlier versions of Neovim, that's a life choice and you're on
your own with `autocmd` and `after/ftplugin` to load the ls. The server should
(theoretically) work from 0.8 onwards.

#### Triggering definition jumps

Both [Jump to model from ref](#jump-to-model-from-ref) and
[Jump to macro definition](#jump-to-macro-definition) go through your editor's
standard definition request, so the same keymap covers both. In nvim that's:

```lua
vim.lsp.buf.definition()
```

My personal config does this with a keymap that uses a Telescope command for
the same effect:

```lua
vim.keymap.set("n", "gd", "<cmd>Telescope lsp_definitions<CR>", { desc = "..." })
```
