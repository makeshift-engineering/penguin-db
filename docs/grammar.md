# SQL Grammar and Syntax Specification

This document provides a technical explanation of the PenguinDB SQL grammar. The complete grammar definitions can be obtained in the following formats:

## Case Insensitivity

All SQL keywords and unquoted identifiers are case-insensitive. For example, keywords such as `SELECT`, `select`, and `SeLeCt` are evaluated identically. Similarly, unquoted table, column, and database names are resolved case-insensitively. String literals enclosed in single quotes preserve their exact character casing.

## BNF Grammar

```bnf
<program>   ::= <statement> | <statement> <program>
<statement> ::= <manipulation_statement> <terminator>

<manipulation_statement> ::= <db_manipulation_statement> | <table_manipulation_statement> | <data_manipulation_statement>

<db_manipulation_statement>    ::= <create_db_statement> | <use_db_statement> | <drop_db_statement>
<table_manipulation_statement> ::= <create_table_statement> | <alter_table_statement> | <drop_table_statement>
<data_manipulation_statement>  ::= <insert_statement> | <select_statement> | <update_statement> | <delete_statement>

<create_db_statement> ::= <keyword_create> <keyword_database> <identifier> | <keyword_create> <keyword_database> <keyword_if> <keyword_not> <keyword_exists> <identifier>
<use_db_statement>    ::= <keyword_use> <identifier>
<drop_db_statement>   ::= <keyword_drop> <keyword_database> <identifier> | <keyword_drop> <keyword_database> <keyword_if> <keyword_exists> <identifier>

<create_table_statement> ::= <keyword_create> <keyword_table> <identifier> <symbol_lparen> <column_definitions> <symbol_rparen>
                           | <keyword_create> <keyword_table> <keyword_if> <keyword_not> <keyword_exists> <identifier> <symbol_lparen> <column_definitions> <symbol_rparen>
<column_definitions>     ::= <column_definition> | <column_definition> <symbol_comma> <column_definitions>

<alter_table_statement> ::= <keyword_alter> <keyword_table> <identifier> <alter_table_action>
<alter_table_action>    ::= <alter_action_add> | <alter_action_modify> | <alter_action_rename> | <alter_action_drop>
<alter_action_add>      ::= <keyword_add> <column_definition> | <keyword_add> <keyword_column> <column_definition>
<alter_action_modify>   ::= <keyword_modify> <column_definition> | <keyword_modify> <keyword_column> <column_definition>
<alter_action_rename>   ::= <keyword_rename> <rename_target>
<rename_target>         ::= <keyword_to> <identifier> | <keyword_column> <identifier> <keyword_to> <identifier>
<alter_action_drop>     ::= <keyword_drop> <keyword_column> <identifier>

<drop_table_statement> ::= <keyword_drop> <keyword_table> <identifier> | <keyword_drop> <keyword_table> <keyword_if> <keyword_exists> <identifier>

<column_definition> ::= <identifier> <data_type> | <identifier> <data_type> <column_constraints>

<column_constraints>       ::= <key_constraint>
                              | <key_constraint>     <after_key_constraint>
                              | <null_constraint>
                              | <null_constraint>    <after_null_constraint>
                              | <default_constraint>
                              | <default_constraint> <after_default_constraint>
                              | <foreign_constraint>

<after_key_constraint>     ::= <null_constraint>
                              | <null_constraint>    <after_null_constraint>
                              | <default_constraint>
                              | <default_constraint> <after_default_constraint>
                              | <foreign_constraint>

<after_null_constraint>    ::= <default_constraint>
                              | <default_constraint> <after_default_constraint>
                              | <foreign_constraint>

<after_default_constraint> ::= <foreign_constraint>

<key_constraint>     ::= <keyword_primary> <keyword_key> | <keyword_unique>
<null_constraint>    ::= <keyword_not> <keyword_null> | <keyword_null>
<default_constraint> ::= <keyword_default> <signed_literal>
<foreign_constraint> ::= <keyword_references> <identifier> <symbol_lparen> <identifier> <symbol_rparen>

<signed_literal> ::= <literal> | <symbol_minus> <numeric_literal> | <symbol_plus> <numeric_literal>

<select_statement> ::= <keyword_select>                   <select_list> <keyword_from> <table_references>
                     | <keyword_select> <select_modifier> <select_list> <keyword_from> <table_references>
                     | <keyword_select>                   <select_list> <keyword_from> <table_references> <select_clauses>
                     | <keyword_select> <select_modifier> <select_list> <keyword_from> <table_references> <select_clauses>

<select_modifier>       ::= <keyword_distinct> | <keyword_all>

<select_clauses>        ::= <where_clause>
                          | <where_clause>    <post_where_clauses>
                          | <group_by_clause>
                          | <group_by_clause> <post_group_by_clauses>
                          | <order_by_clause>
                          | <order_by_clause> <limit_clause>
                          | <limit_clause>

<post_where_clauses>    ::= <group_by_clause>
                          | <group_by_clause> <post_group_by_clauses>
                          | <order_by_clause>
                          | <order_by_clause> <limit_clause>
                          | <limit_clause>

<post_group_by_clauses> ::= <having_clause>
                          | <having_clause>   <post_having_clauses>
                          | <order_by_clause>
                          | <order_by_clause> <limit_clause>
                          | <limit_clause>

<post_having_clauses>   ::= <order_by_clause>
                          | <order_by_clause> <limit_clause>
                          | <limit_clause>

<select_list>       ::= <select_column> | <select_column> <symbol_comma> <select_list>
<select_column>     ::= <symbol_star> | <select_expression> | <select_expression> <keyword_as> <identifier>
<select_expression> ::= <expression> | <condition>

<table_references>   ::= <table_reference> | <table_reference> <symbol_comma> <table_references>
<table_reference>    ::= <table_primary> | <table_primary> <join_clauses>
<table_primary>      ::= <identifier> | <identifier> <keyword_as> <identifier> | <identifier> <identifier>
<join_clauses>       ::= <join_clause> | <join_clause> <join_clauses>
<join_clause>        ::= <keyword_join> <table_primary> <keyword_on> <condition>
                       | <join_type> <keyword_join> <table_primary> <keyword_on> <condition>
                       | <keyword_cross> <keyword_join> <table_primary>
<join_type>          ::= <keyword_inner>
                       | <keyword_left> | <keyword_left> <keyword_outer>
                       | <keyword_right> | <keyword_right> <keyword_outer>
                       | <keyword_full> | <keyword_full> <keyword_outer>

<insert_statement> ::= <keyword_insert> <keyword_into> <identifier> <keyword_values> <value_rows>
                     | <keyword_insert> <keyword_into> <identifier> <symbol_lparen> <column_list> <symbol_rparen> <keyword_values> <value_rows>

<column_list> ::= <identifier> | <identifier> <symbol_comma> <column_list>
<value_rows>  ::= <value_row> | <value_row> <symbol_comma> <value_rows>
<value_row>   ::= <symbol_lparen> <value_list> <symbol_rparen>
<value_list>  ::= <expression> | <expression> <symbol_comma> <value_list>

<update_statement> ::= <keyword_update> <identifier> <keyword_set> <set_list>
                     | <keyword_update> <identifier> <keyword_set> <set_list> <where_clause>

<set_list> ::= <set_item> | <set_item> <symbol_comma> <set_list>
<set_item> ::= <qualified_identifier> <symbol_equal> <expression>

<delete_statement> ::= <keyword_delete> <keyword_from> <identifier>
                     | <keyword_delete> <keyword_from> <identifier> <where_clause>

<where_clause>      ::= <keyword_where> <condition>
<condition>         ::= <or_condition>
<or_condition>      ::= <and_condition> | <and_condition> <keyword_or> <or_condition>
<and_condition>     ::= <not_condition> | <not_condition> <keyword_and> <and_condition>
<not_condition>     ::= <condition_primary> | <keyword_not> <not_condition>
<condition_primary> ::= <predicate> | <symbol_lparen> <condition> <symbol_rparen>
<predicate>         ::= <comparison_predicate> | <like_predicate> | <null_predicate> | <in_predicate> | <between_predicate>

<comparison_predicate> ::= <expression> <comparison_operator> <expression>
<like_predicate>       ::= <expression> <keyword_like> <expression>
                         | <expression> <keyword_not> <keyword_like> <expression>
<null_predicate>       ::= <expression> <keyword_is> <keyword_null>
                         | <expression> <keyword_is> <keyword_not> <keyword_null>
<in_predicate>         ::= <expression> <keyword_in> <symbol_lparen> <value_list> <symbol_rparen>
                         | <expression> <keyword_not> <keyword_in> <symbol_lparen> <value_list> <symbol_rparen>
<between_predicate>    ::= <expression> <keyword_between> <expression> <keyword_and> <expression>
                         | <expression> <keyword_not> <keyword_between> <expression> <keyword_and> <expression>

<comparison_operator> ::= <symbol_equal>
                        | <symbol_not_equal>
                        | <symbol_less_than>
                        | <symbol_greater_than>
                        | <symbol_less_than_or_equal>
                        | <symbol_greater_than_or_equal>

<group_by_clause> ::= <keyword_group> <keyword_by> <group_by_list>
<group_by_list>   ::= <qualified_identifier> | <qualified_identifier> <symbol_comma> <group_by_list>
<having_clause>   ::= <keyword_having> <condition>

<order_by_clause> ::= <keyword_order> <keyword_by> <order_by_list>
<order_by_list>   ::= <order_by_item> | <order_by_item> <symbol_comma> <order_by_list>
<order_by_item>   ::= <expression> | <expression> <keyword_asc> | <expression> <keyword_desc>

<limit_clause> ::= <keyword_limit> <integer_literal>
                 | <keyword_limit> <integer_literal> <keyword_offset> <integer_literal>

<expression> ::= <term> | <term> <symbol_plus> <expression> | <term> <symbol_minus> <expression>
<term>       ::= <factor> | <factor> <symbol_star> <term> | <factor> <symbol_slash> <term> | <factor> <symbol_percent> <term>
<factor>     ::= <literal> | <qualified_identifier> | <function_call>
               | <symbol_lparen> <expression> <symbol_rparen>
               | <symbol_minus> <factor> | <symbol_plus> <factor>

<function_call>    ::= <identifier> <symbol_lparen> <symbol_rparen>
                     | <identifier> <symbol_lparen> <function_args> <symbol_rparen>
<function_args>    ::= <symbol_star>
                     | <expression> | <expression> <symbol_comma> <function_arg_tail>
                     | <keyword_distinct> <expression> | <keyword_distinct> <expression> <symbol_comma> <function_arg_tail>
<function_arg_tail> ::= <expression> | <expression> <symbol_comma> <function_arg_tail>

<qualified_identifier> ::= <identifier> | <identifier> <symbol_dot> <identifier>


<data_type> ::= <keyword_int>
              | <keyword_bigint>
              | <keyword_varchar> <symbol_lparen> <integer_literal> <symbol_rparen>
              | <keyword_boolean>
              | <keyword_text>
              | <keyword_timestamp>

<identifier>      ::= <letter> | <letter> <identifier_tail>
<identifier_tail> ::= <letter> | <digit> | <symbol_underscore>
                    | <letter> <identifier_tail> | <digit> <identifier_tail> | <symbol_underscore> <identifier_tail>

<literal>         ::= <numeric_literal> | <string_literal> | <boolean_literal> | <null_literal>
<null_literal>    ::= <keyword_null>
<boolean_literal> ::= <keyword_true> | <keyword_false>
<numeric_literal> ::= <integer_literal> | <float_literal>
<integer_literal> ::= <digits>
<float_literal>   ::= <digits> <symbol_dot> <digits> | <digits> <symbol_dot> | <symbol_dot> <digits>
<digits>          ::= <digit> | <digit> <digits>
<string_literal>  ::= <symbol_single_quote> <symbol_single_quote>
                    | <symbol_single_quote> <string_content> <symbol_single_quote>
<string_content>  ::= <string_char> | <string_char> <string_content>
<string_char>          ::= <nonquote_character> | <symbol_single_quote> <symbol_single_quote>
<nonquote_character>   ::= (* any character from the source character set except <symbol_single_quote> *)

<letter>           ::= <lowercase_letter> | <uppercase_letter>
<lowercase_letter> ::= "a" | "b" | "c" | "d" | "e" | "f" | "g" | "h" | "i" | "j" | "k" | "l" | "m"
                     | "n" | "o" | "p" | "q" | "r" | "s" | "t" | "u" | "v" | "w" | "x" | "y" | "z"
<uppercase_letter> ::= "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J" | "K" | "L" | "M"
                     | "N" | "O" | "P" | "Q" | "R" | "S" | "T" | "U" | "V" | "W" | "X" | "Y" | "Z"
<digit>            ::= "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"

<keyword_create>     ::= "CREATE"
<keyword_database>   ::= "DATABASE"
<keyword_use>        ::= "USE"
<keyword_drop>       ::= "DROP"
<keyword_if>         ::= "IF"
<keyword_exists>     ::= "EXISTS"

<keyword_table>      ::= "TABLE"
<keyword_alter>      ::= "ALTER"
<keyword_add>        ::= "ADD"
<keyword_column>     ::= "COLUMN"
<keyword_modify>     ::= "MODIFY"
<keyword_rename>     ::= "RENAME"
<keyword_to>         ::= "TO"

<keyword_select>     ::= "SELECT"
<keyword_distinct>   ::= "DISTINCT"
<keyword_all>        ::= "ALL"
<keyword_from>       ::= "FROM"
<keyword_where>      ::= "WHERE"
<keyword_as>         ::= "AS"
<keyword_insert>     ::= "INSERT"
<keyword_into>       ::= "INTO"
<keyword_values>     ::= "VALUES"
<keyword_update>     ::= "UPDATE"
<keyword_set>        ::= "SET"
<keyword_delete>     ::= "DELETE"

<keyword_join>       ::= "JOIN"
<keyword_inner>      ::= "INNER"
<keyword_left>       ::= "LEFT"
<keyword_right>      ::= "RIGHT"
<keyword_full>       ::= "FULL"
<keyword_outer>      ::= "OUTER"
<keyword_cross>      ::= "CROSS"
<keyword_on>         ::= "ON"

<keyword_group>      ::= "GROUP"
<keyword_having>     ::= "HAVING"
<keyword_order>      ::= "ORDER"
<keyword_by>         ::= "BY"
<keyword_asc>        ::= "ASC"
<keyword_desc>       ::= "DESC"
<keyword_limit>      ::= "LIMIT"
<keyword_offset>     ::= "OFFSET"

<keyword_primary>    ::= "PRIMARY"
<keyword_key>        ::= "KEY"
<keyword_not>        ::= "NOT"
<keyword_null>       ::= "NULL"
<keyword_default>    ::= "DEFAULT"
<keyword_unique>     ::= "UNIQUE"
<keyword_references> ::= "REFERENCES"

<keyword_and>        ::= "AND"
<keyword_or>         ::= "OR"
<keyword_true>       ::= "TRUE"
<keyword_false>      ::= "FALSE"
<keyword_like>       ::= "LIKE"
<keyword_is>         ::= "IS"
<keyword_in>         ::= "IN"
<keyword_between>    ::= "BETWEEN"

<keyword_int>        ::= "INT"
<keyword_bigint>     ::= "BIGINT"
<keyword_varchar>    ::= "VARCHAR"
<keyword_boolean>    ::= "BOOLEAN"
<keyword_text>       ::= "TEXT"
<keyword_timestamp>  ::= "TIMESTAMP"

<symbol_lparen>                ::= "("
<symbol_rparen>                ::= ")"
<symbol_comma>                 ::= ","
<symbol_dot>                   ::= "."
<symbol_semicolon>             ::= ";"
<symbol_single_quote>          ::= "'"
<symbol_double_quote>          ::= "\""

<symbol_equal>                 ::= "="
<symbol_not_equal>             ::= "!=" | "<>"
<symbol_less_than>             ::= "<"
<symbol_greater_than>          ::= ">"
<symbol_less_than_or_equal>    ::= "<="
<symbol_greater_than_or_equal> ::= ">="

<symbol_plus>        ::= "+"
<symbol_minus>       ::= "-"
<symbol_star>        ::= "*"
<symbol_slash>       ::= "/"
<symbol_percent>     ::= "%"

<symbol_underscore>  ::= "_"

<terminator> ::= ";"
```

## EBNF Form

A more readable EBNF form of the grammar is given below:

```ebnf
Program   ::= Statement+
Statement ::= ManipulationStatement ';'

ManipulationStatement ::= DbManipulationStatement
                        | TableManipulationStatement
                        | DataManipulationStatement

DbManipulationStatement    ::= 'CREATE' 'DATABASE' ( 'IF' 'NOT' 'EXISTS' )? Identifier
                             | 'DROP' 'DATABASE' ( 'IF' 'EXISTS' )? Identifier
                             | 'USE' Identifier
TableManipulationStatement ::= CreateTableStatement | AlterTableStatement | DropTableStatement
DataManipulationStatement  ::= InsertStatement | SelectStatement | UpdateStatement | DeleteStatement

CreateTableStatement ::= 'CREATE' 'TABLE' ( 'IF' 'NOT' 'EXISTS' )? Identifier '(' ColumnDefinition ( ',' ColumnDefinition )* ')'

AlterTableStatement  ::= 'ALTER' 'TABLE' Identifier AlterAction
AlterAction          ::= ( 'ADD' | 'MODIFY' ) 'COLUMN'? ColumnDefinition
                       | 'RENAME' ( 'TO' Identifier | 'COLUMN' Identifier 'TO' Identifier )
                       | 'DROP' 'COLUMN' Identifier

DropTableStatement   ::= 'DROP' 'TABLE' ( 'IF' 'EXISTS' )? Identifier

ColumnDefinition  ::= Identifier DataType ColumnConstraints?

ColumnConstraints ::= KeyConstraint     NullConstraint? DefaultConstraint? ForeignConstraint?
                    | NullConstraint    DefaultConstraint? ForeignConstraint?
                    | DefaultConstraint ForeignConstraint?
                    | ForeignConstraint

KeyConstraint        ::= 'PRIMARY' 'KEY' | 'UNIQUE'
NullConstraint       ::= 'NOT' 'NULL' | 'NULL'
DefaultConstraint    ::= 'DEFAULT' SignedLiteral
ForeignConstraint    ::= 'REFERENCES' Identifier '(' Identifier ')'

SignedLiteral        ::= Literal | ( '+' | '-' ) NumericLiteral

SelectStatement     ::= 'SELECT' ( 'DISTINCT' | 'ALL' )? SelectList
                        'FROM' TableReference ( ',' TableReference )*
                        WhereClause?
                        GroupByClause?
                        HavingClause?
                        OrderByClause?
                        LimitClause?
SelectList          ::= SelectColumn ( ',' SelectColumn )*
SelectColumn        ::= '*' | SelectExpression ( 'AS' Identifier )?
SelectExpression    ::= Expression | Condition

InsertStatement      ::= 'INSERT' 'INTO' Identifier
                         ( '(' Identifier ( ',' Identifier )* ')' )?
                         'VALUES' ValueRow ( ',' ValueRow )*

ValueRow             ::= '(' Expression ( ',' Expression )* ')'

UpdateStatement      ::= 'UPDATE' Identifier 'SET' SetItem ( ',' SetItem )* WhereClause?
SetItem              ::= QualifiedIdentifier '=' Expression

DeleteStatement      ::= 'DELETE' 'FROM' Identifier WhereClause?

TableReference       ::= TablePrimary ( JoinClause )*
TablePrimary         ::= Identifier ( ( 'AS' )? Identifier )?
JoinClause           ::= JoinType? 'JOIN' TablePrimary 'ON' Condition
JoinType             ::= 'INNER' | 'LEFT' 'OUTER'? | 'RIGHT' 'OUTER'? | 'FULL' 'OUTER'? | 'CROSS'

WhereClause          ::= 'WHERE' Condition
Condition            ::= OrCondition
OrCondition          ::= AndCondition ( 'OR' AndCondition )*
AndCondition         ::= NotCondition ( 'AND' NotCondition )*
NotCondition         ::= ConditionPrimary | 'NOT' NotCondition
ConditionPrimary     ::= Predicate | '(' Condition ')'
Predicate            ::= ComparisonPredicate
                       | LikePredicate
                       | NullPredicate
                       | InPredicate
                       | BetweenPredicate
ComparisonPredicate  ::= Expression ComparisonOperator Expression
LikePredicate        ::= Expression 'NOT'? 'LIKE' Expression
NullPredicate        ::= Expression 'IS' 'NOT'? 'NULL'
InPredicate          ::= Expression 'NOT'? 'IN' '(' Expression ( ',' Expression )* ')'
BetweenPredicate     ::= Expression 'NOT'? 'BETWEEN' Expression 'AND' Expression
ComparisonOperator   ::= '=' | '!=' | '<>' | '<' | '>' | '<=' | '>='

GroupByClause        ::= 'GROUP' 'BY' QualifiedIdentifier ( ',' QualifiedIdentifier )*
HavingClause         ::= 'HAVING' Condition
OrderByClause        ::= 'ORDER' 'BY' OrderByItem ( ',' OrderByItem )*
OrderByItem          ::= Expression ( 'ASC' | 'DESC' )?
LimitClause          ::= 'LIMIT' IntegerLiteral ( 'OFFSET' IntegerLiteral )?

Expression           ::= Term ( ( '+' | '-' ) Term )*
Term                 ::= Factor ( ( '*' | '/' | '%' ) Factor )*
Factor               ::= Literal
                       | QualifiedIdentifier
                       | FunctionCall
                       | '(' Expression ')'
                       | ( '+' | '-' ) Factor

FunctionCall         ::= Identifier '(' FunctionArgs? ')'
FunctionArgs         ::= '*' | ( 'DISTINCT' )? Expression ( ',' Expression )*

QualifiedIdentifier  ::= Identifier ( '.' Identifier )?

DataType             ::= 'INT'
                       | 'BIGINT'
                       | 'VARCHAR' '(' IntegerLiteral ')'
                       | 'BOOLEAN'
                       | 'TEXT'
                       | 'TIMESTAMP'

Identifier           ::= Letter ( Letter | Digit | '_' )*

Literal              ::= NumericLiteral | StringLiteral | BooleanLiteral | NullLiteral
NullLiteral          ::= 'NULL'
BooleanLiteral       ::= 'TRUE' | 'FALSE'
NumericLiteral       ::= IntegerLiteral | FloatLiteral
IntegerLiteral       ::= Digit+
FloatLiteral         ::= Digit+ '.' Digit+ | Digit+ '.' | '.' Digit+
StringLiteral        ::= "'" StringChar* "'"
StringChar           ::= NonQuoteCharacter | "''"
NonQuoteCharacter    ::= (* any character except single-quote *)

Letter               ::= LowercaseLetter | UppercaseLetter
LowercaseLetter      ::= 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h' | 'i' | 'j' | 'k' | 'l' | 'm'
                       | 'n' | 'o' | 'p' | 'q' | 'r' | 's' | 't' | 'u' | 'v' | 'w' | 'x' | 'y' | 'z'
UppercaseLetter      ::= 'A' | 'B' | 'C' | 'D' | 'E' | 'F' | 'G' | 'H' | 'I' | 'J' | 'K' | 'L' | 'M'
                       | 'N' | 'O' | 'P' | 'Q' | 'R' | 'S' | 'T' | 'U' | 'V' | 'W' | 'X' | 'Y' | 'Z'
Digit                ::= '0' | '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9'

```

### Notes

- **Program**: The top-level rule. A program is one or more semicolon-terminated statements, enabling scripts with multiple SQL statements separated by `;`.
- **IF EXISTS / IF NOT EXISTS**: `CREATE DATABASE`, `CREATE TABLE` accept an optional `IF NOT EXISTS` clause to suppress errors when the target already exists. `DROP DATABASE`, `DROP TABLE` accept an optional `IF EXISTS` clause to suppress errors when the target does not exist.
- **Identifier / QualifiedIdentifier**: `Identifier` governs database, table, and column names. Must begin with a letter and may include letters, digits, and underscores. `QualifiedIdentifier` extends this to support dot-separated `table.column` references such as `users.id` or `orders.total`. Qualified identifiers are used in `Factor`, `SetItem`, and `GROUP BY`.
- **Literal**: Denotes fixed data values.
- **NullLiteral / BooleanLiteral**: Captures SQL boolean flags (`TRUE`/`FALSE`) and the missing-data marker (`NULL`).
- **NumericLiteral / IntegerLiteral / FloatLiteral**: Governs integer and fractional digits. `FloatLiteral` accepts all three forms SQL allows: standard (`3.14`), leading-dot (`.14`), and trailing-dot (`10.`). Only `IntegerLiteral` is accepted by `LIMIT`, `OFFSET`, and `VARCHAR`.
- **StringLiteral / NonQuoteCharacter**: Resolves single-quoted text values. An empty string `''` is valid. To embed a literal single quote inside a string, double it: `'it''s'` represents `it's`. Per the SQL standard, `NonQuoteCharacter` is any character from the source character set except the single-quote delimiter. In the grammar this is expressed via `StringChar ::= NonQuoteCharacter | "''"`, where `''` is treated as a single escaped-quote unit by the lexer using a greedy longest-match rule.
- **SignedLiteral**: Supports both unary `+` and `-` for numeric literals in `DEFAULT` values: `DEFAULT -1`, `DEFAULT +5`.
- **SelectStatement**: Supports an optional `DISTINCT` or `ALL` quantifier after `SELECT`, absorbed directly into the four `<select_statement>` alternatives rather than via a nullable rule. The optional clause tail is expressed through four non-nullable helper rules — `<select_clauses>`, `<post_where_clauses>`, `<post_group_by_clauses>`, and `<post_having_clauses>` — each enumerating only the valid non-empty suffixes that may follow a given clause. Together they cover all 23 valid non-empty clause combinations while enforcing canonical ordering (`WHERE → GROUP BY → HAVING → ORDER BY → LIMIT`). `HAVING` is only reachable through `<post_group_by_clauses>`, so `GROUP BY` before `HAVING` is structurally guaranteed. No nullable rules are used anywhere in the BNF.
- **SelectList / SelectColumn / SelectExpression**: Each item in a select list is independently a `SelectColumn`, which can be a bare `*` or any `SelectExpression` with an optional `AS` alias. A `SelectExpression` may be an arithmetic `Expression` or a boolean `Condition`.
- **TableReference / JoinClause**: A `TableReference` is a `TablePrimary` (an identifier with an optional alias) followed by zero or more `JoinClause`s. Supported join types are: `INNER`, `LEFT [OUTER]`, `RIGHT [OUTER]`, `FULL [OUTER]`, and `CROSS`. All non-cross joins require an `ON` condition.
- **Predicate**: The grammar supports five predicate types: `ComparisonPredicate` (`=`, `!=`, `<>`, `<`, `>`, `<=`, `>=`), `LikePredicate` (`LIKE` / `NOT LIKE`), `NullPredicate` (`IS NULL` / `IS NOT NULL`), `InPredicate` (`IN` / `NOT IN`), and `BetweenPredicate` (`BETWEEN ... AND ...` / `NOT BETWEEN ... AND ...`).
- **GROUP BY / HAVING**: `GROUP BY` accepts a comma-separated list of qualified identifiers. `HAVING` filters groups using a condition and may only appear after `GROUP BY`.
- **ORDER BY**: Accepts a comma-separated list of order items. Each item is an expression with an optional `ASC` (ascending, default) or `DESC` (descending) direction.
- **LIMIT / OFFSET**: `LIMIT` restricts the result set size. An optional `OFFSET` clause skips a specified number of rows before returning results. Both accept only `IntegerLiteral` values.
- **FunctionCall**: Supports general function call syntax: `identifier(args)`. Function arguments can be a bare `*` (for `COUNT(*)`), or one or more expressions optionally preceded by `DISTINCT` (for `COUNT(DISTINCT col)`). This covers all standard aggregate functions (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`) and any future scalar functions.
- **ColumnConstraints**: Supports four constraint types — key, null, default, and foreign — each of which may appear at most once per column. Constraints must be written in canonical order: `KeyConstraint` → `NullConstraint` → `DefaultConstraint` → `ForeignConstraint`. The grammar encodes all 15 valid non-empty subsets of these four types in that fixed order. **Parser note**: the parser must verify at semantic analysis time that no constraint type is duplicated; the grammar structure alone enforces canonical ordering but does not prevent a user from writing the same constraint twice if the grammar were extended permissively.
- **NullConstraint**: Accepts both `NOT NULL` and explicit `NULL`. While `NULL` is the default column behavior, explicitly stating it is valid SQL and commonly used in schema definitions.
- **ForeignConstraint**: Column-level referential constraint. Syntax: `REFERENCES table_name (column_name)`, pointing to exactly one column in another table.
- **Letter / Digit**: Fundamental character classes for identifiers.
