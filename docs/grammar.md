# SQL Grammar and Syntax Specification

This document provides a technical explanation of the PenguinDB SQL grammar. The complete grammar definitions can be obtained in the following formats:

- Backus-Naur Form (BNF): [grammar.bnf](./sql-grammar/grammar.bnf)
- Extended Backus-Naur Form (EBNF): [grammar.ebnf](./sql-grammar/grammar.ebnf)

---

## Case Insensitivity

All SQL keywords and unquoted identifiers are case-insensitive. For example, keywords such as `SELECT`, `select`, and `SeLeCt` are evaluated identically. Similarly, unquoted table, column, and database names are resolved case-insensitively. String literals enclosed in single quotes preserve their exact character casing.

---

## Statement Entry Points

### Statement and Manipulation Statement

```ebnf
Statement             ::= ManipulationStatement ';'

ManipulationStatement ::= DbManipulationStatement
                        | TableManipulationStatement
                        | DataManipulationStatement
```

- **Statement**: Defines the ultimate entry point of the parser. It requires a manipulation statement followed by a terminating semicolon, representing a single complete command.
- **ManipulationStatement**: Categorizes the types of executable operations into database, table, and data manipulation rules. This routing assists the parser in delegating subsequent token streams to specific handlers.

---

## Database Manipulation

### Database Manipulation Statement

```ebnf
DbManipulationStatement    ::= ( 'CREATE' | 'DROP' ) 'DATABASE' Identifier | 'USE' Identifier
```

- **CREATE DATABASE / DROP DATABASE**: Defines syntax for creating new databases or deleting existing ones. The parser uses this to trigger file-system level database directory creation or deletion.
- **USE**: Sets the active database context for the current session. Any subsequent table queries will assume this database scope unless explicitly overridden.

---

## Table Schema Manipulation

### Create Table Statement

```ebnf
CreateTableStatement ::= 'CREATE' 'TABLE' Identifier '(' ColumnDefinition ( ',' ColumnDefinition )* ')'
```

- **CreateTableStatement**: Governs schema definition for new tables. It requires a table name (Identifier) followed by a comma-separated list of column definitions enclosed in parentheses. The parser uses this to build the catalog schema.

### Alter Table Statement

```ebnf
AlterTableStatement  ::= 'ALTER' 'TABLE' Identifier AlterAction
AlterAction          ::= ( 'ADD' | 'MODIFY' ) 'COLUMN'? ColumnDefinition
                       | 'RENAME' ( 'TO' Identifier | 'COLUMN' Identifier 'TO' Identifier )
                       | 'DROP' 'COLUMN' Identifier
```

- **AlterTableStatement**: Governs DDL modifications to existing tables.
- **AlterAction**: Defines sub-commands for table modification:
  - `ADD` or `MODIFY`: Adds new columns or alters data types and constraints on existing columns.
  - `RENAME`: Renames the table or a specific column.
  - `DROP COLUMN`: Drops a column from the schema, signaling the storage layer to purge or ignore the associated data.

### Drop Table Statement

```ebnf
DropTableStatement   ::= 'DROP' 'TABLE' Identifier
```

- **DropTableStatement**: Governs table deletion syntax. The execution of this statement instructs the storage engine to drop table files and clean up catalog metadata.

---

## Column Definition and Constraints

### Column Definition and Constraints

```ebnf
ColumnDefinition     ::= Identifier DataType ColumnConstraints?
ColumnConstraints    ::= KeyConstraint
                       | NullConstraint
                       | DefaultConstraint
                       | KeyConstraint     NullConstraint    DefaultConstraint
                       | KeyConstraint     DefaultConstraint NullConstraint
                       | NullConstraint    KeyConstraint     DefaultConstraint
                       | NullConstraint    DefaultConstraint KeyConstraint
                       | DefaultConstraint KeyConstraint     NullConstraint
                       | DefaultConstraint NullConstraint    KeyConstraint
                       | KeyConstraint     NullConstraint
                       | NullConstraint    KeyConstraint
                       | KeyConstraint     DefaultConstraint
                       | DefaultConstraint KeyConstraint
                       | NullConstraint    DefaultConstraint
                       | DefaultConstraint NullConstraint

KeyConstraint        ::= 'PRIMARY' 'KEY' | 'UNIQUE'
NullConstraint       ::= 'NOT' 'NULL'
DefaultConstraint    ::= 'DEFAULT' SignedLiteral
SignedLiteral        ::= Literal | '-' NumericLiteral
```

- **ColumnDefinition**: Associates a column name (Identifier) with a concrete data type and optional constraints.
- **ColumnConstraints**: Models combinations of column constraints. Permuting these options explicitly in the grammar allows the parser to validate constraint ordering without requiring custom AST post-validation logic.
- **KeyConstraint**: Configures uniqueness checks. `PRIMARY KEY` registers the column as the primary key of the table, while `UNIQUE` enforces unique constraints.
- **NullConstraint**: Sets nullability rules. `NOT NULL` prevents null values from being inserted.
- **DefaultConstraint**: Assigns a default value for the column when no value is provided during inserts.
- **SignedLiteral**: Allows literal numbers and values to carry positive or negative signs.

---

## Data Manipulation

### Select Statement

```ebnf
SelectStatement      ::= 'SELECT' SelectList 'FROM' Identifier WhereClause? LimitClause?
SelectList           ::= '*' | SelectColumn ( ',' SelectColumn )*
SelectColumn         ::= Expression ( 'AS' Identifier )?
```

- **SelectStatement**: Defines syntax for retrieving records from a target table. It processes projections, source tables, logical filters, and row limits.
- **SelectList**: Specifies target columns or expressions to project. A wildcard (`*`) denotes all columns.
- **SelectColumn**: Resolves to a column name or an expression, optionally bound to a display alias using `AS`.

### Insert Statement

```ebnf
InsertStatement      ::= 'INSERT' 'INTO' Identifier
                         ( '(' Identifier ( ',' Identifier )* ')' )?
                         'VALUES' ValueRow ( ',' ValueRow )*
ValueRow             ::= '(' Expression ( ',' Expression )* ')'
```

- **InsertStatement**: Specifies the insert interface. It takes a destination table, an optional list of target columns, and rows of values to append.
- **ValueRow**: A grouped tuple of expressions representing the values for a single record.

### Update Statement

```ebnf
UpdateStatement      ::= 'UPDATE' Identifier 'SET' SetItem ( ',' SetItem )* WhereClause?
SetItem              ::= Identifier '=' Expression
```

- **UpdateStatement**: Defines syntax for updating values in existing database rows.
- **SetItem**: A key-value pair associating a target column name with an expression.

### Delete Statement

```ebnf
DeleteStatement      ::= 'DELETE' 'FROM' Identifier WhereClause?
```

- **DeleteStatement**: Outlines row deletion criteria. If a `WhereClause` is absent, it deletes all records from the target table.

---

## Query Clauses and Modifiers

### Where Clause

```ebnf
WhereClause          ::= 'WHERE' Condition
Condition            ::= OrCondition
OrCondition          ::= AndCondition ( 'OR' AndCondition )*
AndCondition         ::= NotCondition ( 'AND' NotCondition )*
NotCondition         ::= ConditionPrimary | 'NOT' NotCondition
ConditionPrimary     ::= Predicate | '(' Condition ')'
Predicate            ::= Expression ComparisonOperator Expression
ComparisonOperator   ::= '=' | '!=' | '<>' | '<' | '>' | '<=' | '>='
```

- **WhereClause**: Restricts the records processed by DML statements based on logical conditions.
- **Condition / OrCondition / AndCondition / NotCondition**: Implements a logical expression parser. Splitting these levels establishes operator precedence for boolean logic, ensuring `AND` binds tighter than `OR` and `NOT` binds tighter than `AND`.
- **ConditionPrimary**: Encapsulates atomic predicates or nested conditional expressions inside parentheses to override precedence.
- **Predicate**: Performs value comparisons.
- **ComparisonOperator**: Matches standard comparison symbols for equality, inequality, and order.

### Limit Clause

```ebnf
LimitClause          ::= 'LIMIT' IntegerLiteral
```

- **LimitClause**: Sets a maximum limit on the number of records returned.

---

## Expressions and Operations

### Expression, Term, and Factor

```ebnf
Expression           ::= Term ( ( '+' | '-' ) Term )*
Term                 ::= Factor ( ( '*' | '/' | '%' ) Factor )*
Factor               ::= Literal | Identifier | '(' Expression ')' | '-' Factor
```

- **Expression / Term / Factor**: Configures mathematical order of operations:
  - `Factor` processes base operands, parenthesized expressions, and negative signs.
  - `Term` evaluates multiplicative operations (`*`, `/`, `%`) which take precedence over additive operations.
  - `Expression` evaluates additive operations (`+`, `-`).

---

## Data Types

### Data Type

```ebnf
DataType             ::= 'INT'
                       | 'BIGINT'
                       | 'VARCHAR' '(' IntegerLiteral ')'
                       | 'BOOLEAN'
                       | 'TEXT'
                       | 'TIMESTAMP'
```

- **DataType**: Enforces field validation. It defines supported column datatypes, including variable-length strings (`VARCHAR` with explicit sizing constraint), fixed types (`INT`, `BIGINT`, `BOOLEAN`, `TEXT`), and date/time markers (`TIMESTAMP`).

---

## Lexical Rules

### Identifier and Literals

```ebnf
Identifier           ::= Letter ( Letter | Digit | '_' )*

Literal              ::= NumericLiteral | StringLiteral | BooleanLiteral | NullLiteral
NullLiteral          ::= 'NULL'
BooleanLiteral       ::= 'TRUE' | 'FALSE'
NumericLiteral       ::= IntegerLiteral | FloatLiteral
IntegerLiteral       ::= Digit+
FloatLiteral         ::= Digit+ '.' Digit+
StringLiteral        ::= "'" Character+ "'"

Letter               ::= LowercaseLetter | UppercaseLetter
LowercaseLetter      ::= 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h' | 'i' | 'j' | 'k' | 'l' | 'm'
                       | 'n' | 'o' | 'p' | 'q' | 'r' | 's' | 't' | 'u' | 'v' | 'w' | 'x' | 'y' | 'z'
UppercaseLetter      ::= 'A' | 'B' | 'C' | 'D' | 'E' | 'F' | 'G' | 'H' | 'I' | 'J' | 'K' | 'L' | 'M'
                       | 'N' | 'O' | 'P' | 'Q' | 'R' | 'S' | 'T' | 'U' | 'V' | 'W' | 'X' | 'Y' | 'Z'
Digit                ::= '0' | '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9'
Character            ::= Letter | Digit | '_' | ' ' | '-' | '@' | '.'
```

- **Identifier**: Governs database, table, and column names. They must begin with a letter and can include letters, digits, and underscores.
- **Literal**: Denotes fixed data values.
- **NullLiteral / BooleanLiteral**: Captures SQL boolean flags (`TRUE`/`FALSE`) and the missing data flag (`NULL`).
- **NumericLiteral / IntegerLiteral / FloatLiteral**: Governs integer and fractional digits.
- **StringLiteral**: Resolves single-quoted character sequences representing raw text values.
- **Letter / Digit / Character**: Fundamental character sets allowed within identifiers and string values.
