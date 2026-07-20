# dbt-ls

A language server (LSP) for dbt projects, written in Go.

## Project scope

dbt-ls provides editor features (completions, go-to-definition, diagnostics)
for dbt Core SQL models and sources. It does not run dbt itself.

## Directory map

```
server/   LSP server lifecycle, message dispatch
analysis/ project parsing, indexing, completions, go-to-definition
lsp/      request/response types, LSP protocol helpers
rpc/      JSON-RPC framing and message decoding
logger/   structured logging utilities
```
