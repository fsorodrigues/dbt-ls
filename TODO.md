1. support dbt source tag
    1. database name completion
    2. table name completion
    - walk all yaml files and find sources properties
        - transform those into searchable data structures (potentially nested
        maps???)

2. support dbt macros
    1. macro name completion
    - track all macros from macros/ directory (or read dbt_project .yml and
    find all directories that contains macros in the macro-path)
        - transform those into a searchable data structure

- Refactor TextDocumentCodeCompletion
    - add a decision method that figures out what kind of completion is being
    requested (needs to parse the line/cursor position)
    - build completion response builder methods
    - call completion response builder accordingly (maybe a switch statement)

3. support column name completion
