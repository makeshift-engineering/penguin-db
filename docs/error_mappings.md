# PenguinDB Error Codes & Diagnostic Mappings

PenguinDB features a compiler-grade diagnostic and error reporting engine inspired by modern language compilers (such as Rust and Clang). Every diagnostic contains:
- **Diagnostic Code** (`[E####]`): A deterministic 4-digit numeric code categorising the exact failure.
- **Source Coordinates** (`Line:Col`): The exact 1-based line and column in the query where the error originated.
- **Category**: A high-level description (`Lexical Error`, `Syntax Error`, `Semantic Error`, `Runtime Error`).
- **Carets & Source Snippet** (`^^^^`): Visual highlighting directly beneath the offending token or expression.
- **Wire Protocol Compatibility**: Full mapping to PostgreSQL wire protocol `ErrorResponse` packets, including standard 5-character **SQLSTATE** codes and 1-based character position offsets (`P#`).

---

## 1. Numbering Scheme & Ranges

Diagnostic codes are grouped into contiguous ranges reflecting the execution pipeline:

| Range | Pipeline Stage | Category | Description |
| :--- | :--- | :--- | :--- |
| **`1000–1999`** | **Lexer** | `Lexical Error` | Unrecognised characters, unterminated string literals, unterminated block comments. |
| **`2000–2999`** | **Parser** | `Syntax Error` | Grammatical violations, unexpected tokens, unexpected end-of-file, malformed statements. |
| **`3000–3999`** | **Planner** | `Semantic Error` | Catalog resolution (unknown database/table/column), type checking, grouping/aggregation validation, ambiguous bindings. |
| **`4000–4999`** | **Executor** | `Runtime Error` | Runtime constraints (NOT NULL, unique primary keys, VARCHAR length bounds), runtime arithmetic (division by zero, integer overflow). |

---

## 2. Lexer Diagnostic Codes (`1000–1999`)

Lexer diagnostics occur during tokenisation before any grammar parsing begins. All lexer errors map to PostgreSQL SQLSTATE `42601` (`syntax_error`).

| Code | Constant Name | SQLSTATE | Description | Example Trigger |
| :--- | :--- | :--- | :--- | :--- |
| `[E1001]` | `CodeUnexpectedChar` | `42601` | Encountered a character not permitted in SQL source outside of string literals (e.g. `$`, `~`, `@`, `#`). | `SELECT * FROM users WHERE id = $;` |
| `[E1002]` | `CodeUnterminatedString` | `42601` | A single-quoted string literal opened at a line/column was never closed before EOF or newline. | `SELECT 'hello FROM users;` |
| `[E1003]` | `CodeUnterminatedComment` | `42601` | A multi-line block comment `/* ...` was opened but never closed with `*/`. | `SELECT /* unclosed comment 1;` |

---

## 3. Parser Diagnostic Codes (`2000–2999`)

Parser diagnostics are produced by the recursive descent parser when token sequences violate SQL grammar rules. All parser errors map to SQLSTATE `42601` (`syntax_error`).

| Code | Constant Name | SQLSTATE | Description | Example Trigger |
| :--- | :--- | :--- | :--- | :--- |
| `[E2001]` | `CodeUnexpectedToken` | `42601` | A token was found where a different token was expected by the grammar. | `SELECT FROM users;` |
| `[E2002]` | `CodeUnexpectedEOF` | `42601` | The query terminated unexpectedly in the middle of a statement or clause. | `SELECT * FROM;` |
| `[E2003]` | `CodeInvalidDataType` | `42601` | An unrecognised type name appeared in column definitions (expected `INT`, `BIGINT`, `FLOAT`, `DOUBLE`, `DECIMAL`, `VARCHAR`, `TEXT`, `BOOLEAN`, `TIMESTAMP`). | `CREATE TABLE t (id BLOB);` |
| `[E2004]` | `CodeInvalidAlterAction` | `42601` | The keyword following `ALTER TABLE <name>` is not one of `ADD`, `MODIFY`, `RENAME`, or `DROP`. | `ALTER TABLE t UPDATE c;` |
| `[E2005]` | `CodeInvalidJoinType` | `42601` | A join qualifier sequence cannot be matched to any recognised join variant (`INNER`, `LEFT`, `RIGHT`, `FULL`, `CROSS`). | `SELECT * FROM a NATURAL JOIN b;` |
| `[E2006]` | `CodeExpectedExpression` | `42601` | An arithmetic or value factor was expected, but no valid literal, column ref, or parenthesis was found. | `SELECT 1 + * FROM t;` |
| `[E2007]` | `CodeExpectedCondition` | `42601` | A conditional expression tail was expected within parentheses, but none was matched. | `SELECT * FROM t WHERE ();` |
| `[E2008]` | `CodeInvalidIntegerLiteral` | `42601` | An integer literal could not be parsed into a numeric representation. | `SELECT 999999999999999999999999999999;` |
| `[E2009]` | `CodeMalformedStatement` | `42601` | The input does not begin with any recognisable SQL statement keyword, or `CREATE`/`DROP` is not followed by `DATABASE` or `TABLE`. | `MALFORMED STATEMENT;` |

---

## 4. Planner Diagnostic Codes (`3000–3999`)

Planner diagnostics occur during AST lowering, scope resolution, type validation, and relational plan construction. They are mapped to specific SQLSTATE codes based on standard SQL classification.

| Code | Constant Name | SQLSTATE | Category / Meaning | Description | Example Trigger |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `[E3001]` | `CodeNoActiveDatabase` | `3D000` | `invalid_catalog_name` | No database qualifier was provided and no session database has been selected via `USE`. | `SELECT * FROM users;` *(without active DB)* |
| `[E3002]` | `CodeUnknownDatabase` | `3D000` | `invalid_catalog_name` | The specified database qualifier does not exist in the catalog. | `SELECT * FROM non_existent_db.t;` |
| `[E3003]` | `CodeUnknownTable` | `42P01` | `undefined_table` | The table does not exist in the specified or active database. | `SELECT * FROM missing_table;` |
| `[E3004]` | `CodeDuplicateTableBinding` | `42712` | `duplicate_alias` | Multiple tables or aliases in the `FROM` clause resolve to the same name. | `SELECT * FROM users u, orders u;` |
| `[E3005]` | `CodeUnknownTableBinding` | `42P01` | `undefined_table` | A column reference uses a table alias or qualifier that is not present in scope. | `SELECT bad_alias.id FROM users u;` |
| `[E3006]` | `CodeUnknownColumn` | `42703` | `undefined_column` | The column does not exist on the qualified table or in any table in scope. | `SELECT users.fake_col FROM users;` |
| `[E3007]` | `CodeAmbiguousColumn` | `42702` | `ambiguous_column` | An unqualified column name exists in multiple tables currently in scope. | `SELECT id FROM users JOIN orders ON ...;` |
| `[E3008]` | `CodeUnsupportedJoinType` | `0A000` | `feature_not_supported` | A join type parsed by grammar is not supported by the planner engine. | `SELECT * FROM a FULL OUTER JOIN b;` |
| `[E3009]` | `CodeUnknownFunction` | `42883` | `undefined_function` | A function name is not recognised (standard aggregates: `COUNT`, `SUM`, `AVG`, `MIN`, `MAX`). | `SELECT FOOBAR(col) FROM t;` |
| `[E3010]` | `CodeInvalidFunctionArgs` | `42804` | `datatype_mismatch` | An aggregate function received incorrect arguments or arity (e.g. `SUM(*)` or wrong type). | `SELECT SUM(*) FROM t;` |
| `[E3011]` | `CodeTypeMismatch` | `42804` | `datatype_mismatch` | Incompatible operand types in comparison or binary operation. | `SELECT * FROM t WHERE 'text' = 123;` |
| `[E3012]` | `CodeNonNumericOperand` | `42804` | `datatype_mismatch` | Arithmetic operator applied to a non-numeric, non-NULL operand. | `SELECT 'abc' + 'def';` |
| `[E3013]` | `CodeNonBooleanCondition` | `42804` | `datatype_mismatch` | An expression in a boolean context (`WHERE`, `HAVING`) does not evaluate to boolean. | `SELECT * FROM t WHERE id + 1;` |
| `[E3014]` | `CodeLiteralOverflow` | `22003` | `numeric_value_out_of_range` | A numeric literal exceeds the 64-bit integer or floating-point range. | `SELECT 9223372036854775808;` |
| `[E3015]` | `CodeNonStringOperand` | `42804` | `datatype_mismatch` | `LIKE` applied to a non-string column or pattern. | `SELECT * FROM t WHERE id LIKE '%test%';` |
| `[E3016]` | `CodeUnsupportedExpression` | `0A000` | `feature_not_supported` | Expression type not recognised by the expression resolver. | Complex AST node unsupported in v1 |
| `[E3017]` | `CodeUnsupportedCondition` | `0A000` | `feature_not_supported` | Condition type not recognised by the condition resolver. | Conditional AST node unsupported in v1 |
| `[E3018]` | `CodeDuplicateColumn` | `42701` | `duplicate_column` | `CREATE TABLE` or `ALTER TABLE ADD` contains duplicate column names. | `CREATE TABLE t (id INT, id TEXT);` |
| `[E3019]` | `CodeInvalidDefaultValue` | `22P02` | `invalid_text_representation` | The specified column default value cannot be converted to the column's data type. | `CREATE TABLE t (id INT DEFAULT 'abc');` |
| `[E3020]` | `CodeUnknownAlterColumn` *(alias: `CodeColumnNotFound`)* | `42703` | `undefined_column` | `ALTER TABLE DROP/MODIFY` targets a column that does not exist on the table. | `ALTER TABLE t DROP COLUMN non_existent;` |
| `[E3021]` | `CodeCannotDropPKColumn` | `42P16` | `invalid_table_definition` | `ALTER TABLE DROP COLUMN` attempts to drop a column participating in the primary key. | `ALTER TABLE t DROP COLUMN id;` |
| `[E3022]` | `CodeDuplicateDatabase` *(alias: `CodeDatabaseExists`)* | `42P04` | `duplicate_database` | `CREATE DATABASE` without `IF NOT EXISTS` for an existing database. | `CREATE DATABASE existing_db;` |
| `[E3023]` | `CodeDuplicateTable` *(alias: `CodeTableExists`)* | `42P07` | `duplicate_table` | `CREATE TABLE` without `IF NOT EXISTS` for an existing table. | `CREATE TABLE existing_t (id INT);` |
| `[E3024]` | `CodeUnsupportedDataType` | `0A000` | `feature_not_supported` | Data type requested with unsupported modifiers (e.g. precision/scale). | Custom column type modifiers |
| `[E3025]` | `CodeUnsupportedConstraint` | `0A000` | `feature_not_supported` | Constraint type not supported in current engine version. | Unsupported constraint specification |
| `[E3026]` | `CodeUnsupportedAlter` | `0A000` | `feature_not_supported` | `ALTER TABLE` variant that cannot be applied directly in storage. | `ALTER TABLE t MODIFY id VARCHAR(10);` |
| `[E3027]` | `CodeInvalidAlterAction` | `42601` | `syntax_error` | Alter action not supported by semantic planner. | Invalid alter directive |
| `[E3028]` | `CodeColumnCountMismatch` | `42611` | `invalid_column_definition` | Number of values in `INSERT` does not match declared or target columns. | `INSERT INTO t (id, name) VALUES (1);` |
| `[E3029]` | `CodeUnknownInsertColumn` | `42703` | `undefined_column` | Column in `INSERT (col1, ...)` does not exist on target table. | `INSERT INTO t (ghost) VALUES (1);` |
| `[E3030]` | `CodeDuplicateInsertColumn` | `42701` | `duplicate_column` | `INSERT` target list repeats the same column name multiple times. | `INSERT INTO t (id, id) VALUES (1, 2);` |
| `[E3031]` | `CodeAggregateInWhere` | `42803` | `grouping_error` | An aggregate function was used in a `WHERE` clause (aggregates belong in `HAVING`). | `SELECT * FROM t WHERE COUNT(*) > 1;` |
| `[E3032]` | `CodeMissingGroupBy` | `42803` | `grouping_error` | Unaggregated column in `SELECT` list when query uses aggregation without `GROUP BY`. | `SELECT name, COUNT(*) FROM t;` |
| `[E3033]` | `CodeAggregateWithoutFrom` | `42803` | `grouping_error` | Aggregate used in a query with no `FROM` clause. | `SELECT COUNT(*);` |
| `[E3034]` | `CodeWhereWithoutFrom` | `42P10` | `invalid_column_reference` | `WHERE` clause specified in a query without a `FROM` clause. | `SELECT 1 WHERE 1 = 1;` |
| `[E3035]` | `CodeEmptyStarExpansion` | `42P10` | `invalid_column_reference` | Wildcard `*` used in a query with no tables in scope. | `SELECT *;` |
| `[E3036]` | `CodeOrdinalOutOfRange` | `42P10` | `invalid_column_reference` | Positional ordinal in `ORDER BY` or `GROUP BY` is `< 1` or exceeds column count. | `SELECT a, b FROM t ORDER BY 3;` |
| `[E3037]` | `CodeAggregateInJoin` | `42803` | `grouping_error` | Aggregate function used in a `JOIN ... ON` condition. | `SELECT * FROM a JOIN b ON COUNT(a.id) = 1;` |
| `[E3038]` | `CodeNonOrderableType` | `42804` | `datatype_mismatch` | An unorderable type was used in `ORDER BY` or range comparison. | `SELECT b FROM t ORDER BY bool_col;` |
| `[E3039]` | `CodeInvalidForeignKey` | `42830` | `invalid_foreign_key` | Foreign key references a table or column that does not exist or lacks a primary key. | Foreign key to missing target |
| `[E3040]` | `CodeDuplicateSetColumn` | `42701` | `duplicate_column` | An `UPDATE` statement assigns the same column more than once. | `UPDATE t SET a = 1, a = 2;` |
| `[E3041]` | `CodeNegativeLimit` | `2201W` | `invalid_limit_value` | Negative value supplied to `LIMIT` or `OFFSET`. | `SELECT * FROM t LIMIT -5;` |
| `[E3042]` | `CodeInvalidInsertPlan` | `XX000` | `internal_error` | `INSERT` plan contains invalid rows/source combination. | Malformed planner construction |
| `[E3043]` | `CodeNestedAggregate` | `42803` | `grouping_error` | An aggregate function is nested inside another aggregate function. | `SELECT SUM(COUNT(*)) FROM t;` |
| `[E3044]` | `CodeNullComparison` | `01003` | `warning: null_eliminated` | Warning: `col = NULL` or `col != NULL` will always evaluate to `UNKNOWN`. | `SELECT * FROM t WHERE id = NULL;` |
| `[E3045]` | `CodeNullInList` | `01003` | `warning: null_eliminated` | Warning: `NULL` literal inside `IN (...)` or `NOT IN (...)` list. | `SELECT * FROM t WHERE id IN (1, NULL);` |
| `[E3046]` | `CodeNullAggregate` | `01003` | `warning: null_eliminated` | Warning: Aggregate function invoked with a `NULL` literal argument. | `SELECT SUM(NULL) FROM t;` |
| `[E3047]` | `CodeMalformedAST` | `XX000` | `internal_error` | AST node missing required operands or payload. | Parser AST generation bug |

---

## 5. Executor Diagnostic Codes (`4000–4999`)

Executor diagnostics occur at runtime when queries or data modifications interact with storage, constraints, or runtime arithmetic.

| Code | Constant Name | SQLSTATE | Category / Meaning | Description | Example Trigger |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `[E4001]` | `CodeNullPrimaryKey` | `23502` | `not_null_violation` | Attempted to insert or update a primary key column with a `NULL` value. | `INSERT INTO t (id) VALUES (NULL);` |
| `[E4002]` | `CodeNotNullViolation` | `23502` | `not_null_violation` | Attempted to write `NULL` into a column declared with a `NOT NULL` constraint. | `INSERT INTO users (id, name) VALUES (1, NULL);` |
| `[E4003]` | `CodeDuplicateKey` | `23505` | `unique_violation` | `INSERT` key already exists in the table, or duplicate keys exist within a batch `INSERT`. | `INSERT INTO users VALUES (1); INSERT INTO users VALUES (1);` |
| `[E4004]` | `CodeTypeMismatch` | `42804` | `datatype_mismatch` | Value cannot be converted or coerced to the target column's storage type at runtime. | `INSERT INTO t (int_col) VALUES ('not_an_int');` |
| `[E4005]` | `CodeDivisionByZero` | `22012` | `division_by_zero` | A division (`/`) or modulo (`%`) operation encountered a zero denominator. | `SELECT a / 0 FROM t;` |
| `[E4006]` | `CodeUnsupportedPlan` | `0A000` | `feature_not_supported` | Executor received a plan node type not implemented in the execution engine. | Internal unsupported plan variant |
| `[E4007]` | `CodeVarcharTooLong` | `22001` | `string_data_right_truncation` | String value exceeds the declared `VARCHAR(N)` maximum unicode rune length. | `INSERT INTO t (v5) VALUES ('toolongstring');` |
| `[E4008]` | `CodeUnsupportedExpression` *(alias: `CodeUnsupportedExpr`)* | `0A000` | `feature_not_supported` | An expression variant reached execution that the expression evaluator does not support. | Non-evaluated node in scalar context |
| `[E4009]` | `CodeUnsupportedCondition` *(alias: `CodeUnsupportedCond`)* | `0A000` | `feature_not_supported` | Condition evaluator received an unsupported boolean condition node. | Internal condition unhandled |
| `[E4010]` | `CodeUnsupportedRelationalNode` *(alias: `CodeUnsupportedRelNode`)* | `0A000` | `feature_not_supported` | Query execution tree contains an unknown or unhandled relational node. | Relational node unhandled |
| `[E4011]` | `CodeUnsupportedAggregateFunction` *(alias: `CodeUnsupportedAggFunc`)* | `42883` | `undefined_function` | An aggregate function name is not recognised by the aggregate evaluator. | Custom aggregate function |
| `[E4012]` | `CodeColumnCountMismatch` | `42611` | `invalid_column_definition` | Number of columns supplied at runtime does not match target schema. | `INSERT INTO t SELECT 1, 2, 3;` *(when t has 2 cols)* |
| `[E4013]` | `CodeInvalidLikePattern` | `22025` | `invalid_escape_sequence` | `LIKE` pattern evaluated at runtime to a non-string value. | `SELECT * FROM t WHERE name LIKE 123;` |
| `[E4014]` | `CodeNumericOverflow` | `22003` | `numeric_value_out_of_range` | Integer arithmetic or negation exceeded the supported 32-bit or 64-bit integer range. | `UPDATE t SET id = id + 1;` *(when id is MaxInt32)* |

---

## 6. PostgreSQL Wire Protocol Response Mapping

PenguinDB communicates with standard PostgreSQL clients (`psql`, Python `psycopg2`, Go `pgx`, Node `pg`, etc.) using the Postgres v3 pgwire protocol.

When an error occurs, PenguinDB returns an `ErrorResponse` (`'E'`) packet containing the following fields:

| Field Identifier | Name | Source in PenguinDB | Example Value |
| :---: | :--- | :--- | :--- |
| **`S`** | Severity | `SeverityError`, `SeverityWarning`, `SeverityHint` | `ERROR` |
| **`C`** | SQLSTATE Code | `diagCodeToSQLSTATE(diag.Code)` | `22012`, `23505`, `42601`, `42P01` |
| **`M`** | Primary Message | `[E####] <Category>: <Description>` | `1:13: [E4005] Runtime Error: division by zero` |
| **`D`** | Detail | Visual snippet with carets highlighting offending token | `  SELECT 10 / 0;\n                ^^` |
| **`P`** | Character Position | 1-based character offset of the offending token | `13` |

### Visual Caret Rendering in Clients
Clients such as `psql` and the PenguinDB interactive CLI render carets directly under the offending query segment using `Position` (`P`) and `Detail` (`D`):

```text
ERROR:  1:13: [E4005] Runtime Error: division by zero
LINE 1: SELECT 10 / 0;
                    ^
DETAIL:   SELECT 10 / 0;
                    ^^
```

---

## 7. Global Naming Template & Taxonomy

PenguinDB enforces a strict, harmonized naming architecture across user-facing diagnostics, wire protocol responses, and internal Go identifiers.

### A. The Two Code Systems: Why Characters Appear in SQLSTATE

A frequent question is why error codes like `42P01` or `22P02` have characters between numbers, whereas `[E3003]` and `[E4005]` do not:

1. **PenguinDB Diagnostic Codes (`[E####]`)**:
   - Strictly uniform compiler format: `[E<Stage><Index2D>]`.
   - Always an uppercase `E` followed by exactly 4 decimal digits (`1001` through `4999`).
   - Never contains embedded letters.
2. **PostgreSQL Wire SQLSTATE Codes (e.g. `42P01`, `22P02`, `2201W`, `42601`)**:
   - Defined by the ANSI SQL-92 specification as 5 alphanumeric characters: 2 characters for the **Class** and 3 characters for the **Subclass**.
   - Standard ANSI codes use numeric digits for standard conditions (e.g. `42601` for syntax error, `23505` for unique violation, `22012` for division by zero).
   - **PostgreSQL Vendor Extension Rule (`'P'`)**: The 1992 standard did not define standard codes for engine-specific conditions such as "table does not exist" or "duplicate table". PostgreSQL adopted the convention that **any PostgreSQL-specific error code places the character `'P'` as the first character of the subclass (the 3rd character overall)**:
     - `42` (Access Rule Violation) + `P01` (Postgres Undefined Table) = `42P01`.
     - `42` (Access Rule Violation) + `P04` (Postgres Duplicate Database) = `42P04`.
     - `42` (Access Rule Violation) + `P07` (Postgres Duplicate Table) = `42P07`.
     - `22` (Data Exception) + `P02` (Postgres Invalid Text Representation) = `22P02`.
   - **Standard Limits (`'W'`)**: ANSI SQL:2008 added `2201W` for invalid limit counts.
   - PenguinDB preserves these exact wire codes so external database drivers, ORMs, and `psql` correctly detect specific failure modes.

### B. Internal Go Constant Taxonomy

All Go constants follow a deterministic, category-prefixed taxonomy across packages:

| Prefix Family | Meaning / Semantic Trigger | Examples |
| :--- | :--- | :--- |
| **`CodeUnexpected...`** | Tokenizer or parser found an illegal or out-of-order symbol. | `CodeUnexpectedChar`, `CodeUnexpectedToken`, `CodeUnexpectedEOF` |
| **`CodeExpected...`** | Parser expected a grammatical construct that was missing. | `CodeExpectedExpression`, `CodeExpectedCondition` |
| **`CodeUnterminated...`** | Lexical literal or block was opened but never closed. | `CodeUnterminatedString`, `CodeUnterminatedComment` |
| **`CodeUnknown...`** | Identifier cannot be resolved in catalog or scope. | `CodeUnknownDatabase`, `CodeUnknownTable`, `CodeUnknownColumn`, `CodeUnknownTableBinding`, `CodeUnknownFunction`, `CodeUnknownAlterColumn`, `CodeUnknownInsertColumn` |
| **`CodeDuplicate...`** | Object or name already exists in catalog or statement scope. | `CodeDuplicateDatabase`, `CodeDuplicateTable`, `CodeDuplicateColumn`, `CodeDuplicateTableBinding`, `CodeDuplicateInsertColumn`, `CodeDuplicateSetColumn`, `CodeDuplicateKey` |
| **`CodeInvalid...`** | Construct is structurally or semantically malformed. | `CodeInvalidDataType`, `CodeInvalidAlterAction`, `CodeInvalidJoinType`, `CodeInvalidIntegerLiteral`, `CodeInvalidDefaultValue`, `CodeInvalidForeignKey`, `CodeInvalidInsertPlan`, `CodeInvalidLikePattern` |
| **`CodeUnsupported...`** | Valid SQL syntax that the current storage or execution engine does not yet implement. | `CodeUnsupportedJoinType`, `CodeUnsupportedDataType`, `CodeUnsupportedConstraint`, `CodeUnsupportedAlter`, `CodeUnsupportedPlan`, `CodeUnsupportedExpression`, `CodeUnsupportedCondition`, `CodeUnsupportedRelationalNode`, `CodeUnsupportedAggregateFunction` |
| **`Code<Clause>WithoutFrom`** | Clause illegal without a table source. | `CodeAggregateWithoutFrom`, `CodeWhereWithoutFrom` |
| **`CodeNull...`** | Semantic warnings for three-valued NULL logic anomalies. | `CodeNullComparison`, `CodeNullInList`, `CodeNullAggregate` |
