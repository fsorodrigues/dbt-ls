# dbt LSP

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
project). I don't intend to address SQL syntax diagnostics or do anything
particularly fancy at this stage. There are solid SQL-specific LSP and linters
out there that can be used for those purposes (and in combination with this
project).

### Model name completion

Autocompletes dbt model names inside `ref('...')` macros. The LSP scans your
dbt project for models (currently defaulting to `.sql` extension files) and
maintains an index. When you are typing inside a `ref` call (e.g.,
`ref('my_mod`)`), it suggests available models matching the input.

### Jump to model from ref

Enables "Go to Definition" functionality for dbt models. Triggering your
editor's definition jump command while the cursor is on a model name inside a
`ref('...')` macro will open the corresponding model's source file.

## Requirements

- **Go**: >= 1.25
- **Neovim**: >= 0.8

## Installation

### Compiling from source

```bash
# clone repository & go to directory
git clone https://github.com/fsorodrigues/dbt-lsp
cd dbt-lsp

# compile with go & output binary to bin/ directory (relative path)
go build -o bin/dbt-lsp .

# Optionally move to a directory in your PATH
mv bin/dbt-lsp /usr/local/bin/
```

## Editor Configuration

### Neovim

If you're using Neovim >= 0.11, the lsp can be easily configured by adding the
following to your `init.lua` or a dedicated configuration file. (this uses the
native `vim.lsp.config` and `vim.lsp.enable` APIs). 

```lua
vim.lsp.config('dbt_lsp', {
  cmd = {
    "/path/to/your/dbt_lsp_binary", -- i.e. /usr/local/bin/dbt_lsp
    "--log-file",
    "/path/to/your/dbt_lsp_log_file.txt", -- i.e. /var/log/dbt-lsp/log.txt
    "--log-level",
    "debug", -- "info" for less verbose (but please use debug to help catch bugs and improve this)
  },
  filetypes = { "sql" },
  root_markers = { "dbt_project.yml", "dbt_project.yaml", ".git" },
})

vim.lsp.enable({
    -- other lsps ...
    'dbt_lsp',
    -- ... other lsps
})
```

If you're on earlier versions of Neovim, that's a life choice and you're on
your own with `autocmd` and `after/ftplugin` to load the lsp. The server should
(theoretically) work from 0.8 onwards.
