# SQL Grammar and Syntax Specification

This document provides a technical explanation of the PenguinDB SQL grammar. The complete grammar definitions can be obtained in the following formats:

## Case Insensitivity

All SQL keywords and unquoted identifiers are case-insensitive. For example, keywords such as `SELECT`, `select`, and `SeLeCt` are evaluated identically. Similarly, unquoted table, column, and database names are resolved case-insensitively. String literals enclosed in single quotes preserve their exact character casing.

## BNF Grammar

```bnf
<statement> ::= <manipulation_statement> <terminator>
<manipulation_statement> ::= <db_manipulation_statement> | <table_manipulation_statement> | <data_manipulation_statement>

<db_manipulation_statement> ::= <create_db_statement> | <use_db_statement> | <drop_db_statement>
<table_manipulation_statement> ::= <create_table_statement> | <alter_table_statement> | <drop_table_statement>
<data_manipulation_statement> ::= <insert_statement> | <select_statement> | <update_statement> | <delete_statement>

<create_db_statement> ::= <keyword_create> <keyword_database> <identifier>
<use_db_statement> ::= <keyword_use> <identifier>
<drop_db_statement> ::= <keyword_drop> <keyword_database> <identifier>

<create_table_statement> ::= <keyword_create> <keyword_table> <identifier> <symbol_lparen> <column_definitions> <symbol_rparen>
<column_definitions>     ::= <column_definition> | <column_definition> <symbol_comma> <column_definitions>

<alter_table_statement> ::= <keyword_alter> <keyword_table> <identifier> <alter_table_action>
<alter_table_action>    ::= <alter_action_add> | <alter_action_modify> | <alter_action_rename> | <alter_action_drop>
<alter_action_add>      ::= <keyword_add> <column_definition> | <keyword_add> <keyword_column> <column_definition>
<alter_action_modify>   ::= <keyword_modify> <column_definition> | <keyword_modify> <keyword_column> <column_definition>
<alter_action_rename>   ::= <keyword_rename> <rename_target>
<rename_target>         ::= <keyword_to> <identifier> | <keyword_column> <identifier> <keyword_to> <identifier>
<alter_action_drop>     ::= <keyword_drop> <keyword_column> <identifier>

<drop_table_statement> ::= <keyword_drop> <keyword_table> <identifier>

<column_definition>  ::= <identifier> <data_type> | <identifier> <data_type> <column_constraints>

<column_constraints> ::= <key_constraint>
                       | <null_constraint>
                       | <default_constraint>
                       | <key_constraint>     <null_constraint>    <default_constraint>
                       | <key_constraint>     <default_constraint> <null_constraint>
                       | <null_constraint>    <key_constraint>     <default_constraint>
                       | <null_constraint>    <default_constraint> <key_constraint>
                       | <default_constraint> <key_constraint>     <null_constraint>
                       | <default_constraint> <null_constraint>    <key_constraint>
                       | <key_constraint>     <null_constraint>
                       | <null_constraint>    <key_constraint>
                       | <key_constraint>     <default_constraint>
                       | <default_constraint> <key_constraint>
                       | <null_constraint>    <default_constraint>
                       | <default_constraint> <null_constraint>

<key_constraint>     ::= <keyword_primary> <keyword_key> | <keyword_unique>
<null_constraint>    ::= <keyword_not> <keyword_null>
<default_constraint> ::= <keyword_default> <signed_literal>

<signed_literal> ::= <literal> | <symbol_minus> <numeric_literal>

<select_statement> ::= <keyword_select> <select_list> <keyword_from> <identifier>
                     | <keyword_select> <select_list> <keyword_from> <identifier> <where_clause>
                     | <keyword_select> <select_list> <keyword_from> <identifier> <limit_clause>
                     | <keyword_select> <select_list> <keyword_from> <identifier> <where_clause> <limit_clause>

<select_list>    ::= <symbol_star> | <select_columns>
<select_columns> ::= <select_column> | <select_column> <symbol_comma> <select_columns>
<select_column>  ::= <expression> | <expression> <keyword_as> <identifier>

<insert_statement> ::= <keyword_insert> <keyword_into> <identifier> <keyword_values> <value_rows>
                     | <keyword_insert> <keyword_into> <identifier> <symbol_lparen> <column_list> <symbol_rparen> <keyword_values> <value_rows>

<column_list> ::= <identifier> | <identifier> <symbol_comma> <column_list>
<value_rows>  ::= <value_row> | <value_row> <symbol_comma> <value_rows>
<value_row>   ::= <symbol_lparen> <value_list> <symbol_rparen>
<value_list>  ::= <expression> | <expression> <symbol_comma> <value_list>

<update_statement> ::= <keyword_update> <identifier> <keyword_set> <set_list>
                     | <keyword_update> <identifier> <keyword_set> <set_list> <where_clause>

<set_list> ::= <set_item> | <set_item> <symbol_comma> <set_list>
<set_item> ::= <identifier> <symbol_equal> <expression>

<delete_statement> ::= <keyword_delete> <keyword_from> <identifier>
                     | <keyword_delete> <keyword_from> <identifier> <where_clause>

<where_clause>      ::= <keyword_where> <condition>
<condition>         ::= <or_condition>
<or_condition>      ::= <and_condition> | <and_condition> <keyword_or> <or_condition>
<and_condition>     ::= <not_condition> | <not_condition> <keyword_and> <and_condition>
<not_condition>     ::= <condition_primary> | <keyword_not> <not_condition>
<condition_primary> ::= <predicate> | <symbol_lparen> <condition> <symbol_rparen>
<predicate>         ::= <expression> <comparison_operator> <expression>

<comparison_operator> ::= <symbol_equal>
                        | <symbol_not_equal>
                        | <symbol_less_than>
                        | <symbol_greater_than>
                        | <symbol_less_than_or_equal>
                        | <symbol_greater_than_or_equal>

<limit_clause> ::= <keyword_limit> <integer_literal>

<expression> ::= <term> | <term> <symbol_plus> <expression> | <term> <symbol_minus> <expression>
<term>       ::= <factor> | <factor> <symbol_star> <term> | <factor> <symbol_slash> <term> | <factor> <symbol_percent> <term>
<factor>     ::= <literal> | <identifier> | <symbol_lparen> <expression> <symbol_rparen> | <symbol_minus> <factor>

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
<float_literal>   ::= <digits> <symbol_dot> <digits>
<digits>          ::= <digit> | <digit> <digits>
<string_literal>  ::= <symbol_single_quote> <characters> <symbol_single_quote>
<characters>      ::= <character> | <character> <characters>

<letter>           ::= <lowercase_letter> | <uppercase_letter>
<lowercase_letter> ::= "a" | "b" | "c" | "d" | "e" | "f" | "g" | "h" | "i" | "j" | "k" | "l" | "m"
                     | "n" | "o" | "p" | "q" | "r" | "s" | "t" | "u" | "v" | "w" | "x" | "y" | "z"
<uppercase_letter> ::= "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J" | "K" | "L" | "M"
                     | "N" | "O" | "P" | "Q" | "R" | "S" | "T" | "U" | "V" | "W" | "X" | "Y" | "Z"
<digit>            ::= "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
<character>        ::= <letter> | <digit> | "_" | " " | "-" | "@" | "."

<keyword_create>    ::= "CREATE"
<keyword_database>  ::= "DATABASE"
<keyword_schema>    ::= "SCHEMA"
<keyword_use>       ::= "USE"
<keyword_drop>      ::= "DROP"

<keyword_table>     ::= "TABLE"
<keyword_alter>     ::= "ALTER"
<keyword_add>       ::= "ADD"
<keyword_column>    ::= "COLUMN"
<keyword_modify>    ::= "MODIFY"
<keyword_rename>    ::= "RENAME"
<keyword_to>        ::= "TO"

<keyword_select>    ::= "SELECT"
<keyword_from>      ::= "FROM"
<keyword_where>     ::= "WHERE"
<keyword_limit>     ::= "LIMIT"
<keyword_as>        ::= "AS"
<keyword_insert>    ::= "INSERT"
<keyword_into>      ::= "INTO"
<keyword_values>    ::= "VALUES"
<keyword_update>    ::= "UPDATE"
<keyword_set>       ::= "SET"
<keyword_delete>    ::= "DELETE"

<keyword_primary>   ::= "PRIMARY"
<keyword_key>       ::= "KEY"
<keyword_not>       ::= "NOT"
<keyword_null>      ::= "NULL"
<keyword_default>   ::= "DEFAULT"
<keyword_unique>    ::= "UNIQUE"

<keyword_and>       ::= "AND"
<keyword_or>        ::= "OR"
<keyword_true>      ::= "TRUE"
<keyword_false>     ::= "FALSE"

<keyword_int>       ::= "INT"
<keyword_bigint>    ::= "BIGINT"
<keyword_varchar>   ::= "VARCHAR"
<keyword_boolean>   ::= "BOOLEAN"
<keyword_text>      ::= "TEXT"
<keyword_timestamp> ::= "TIMESTAMP"

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

<symbol_plus>       ::= "+"
<symbol_minus>      ::= "-"
<symbol_star>       ::= "*"
<symbol_slash>      ::= "/"
<symbol_percent>    ::= "%"

<symbol_underscore> ::= "_"

<terminator> ::= ";"

```

## EBNF Form

A more readable EBNF form of the grammar is given below:

```ebnf
Statement             ::= ManipulationStatement ';'

ManipulationStatement ::= DbManipulationStatement
                        | TableManipulationStatement
                        | DataManipulationStatement

DbManipulationStatement    ::= ( 'CREATE' | 'DROP' ) 'DATABASE' Identifier | 'USE' Identifier
TableManipulationStatement ::= CreateTableStatement | AlterTableStatement | DropTableStatement
DataManipulationStatement  ::= InsertStatement | SelectStatement | UpdateStatement | DeleteStatement

CreateTableStatement ::= 'CREATE' 'TABLE' Identifier '(' ColumnDefinition ( ',' ColumnDefinition )* ')'

AlterTableStatement  ::= 'ALTER' 'TABLE' Identifier AlterAction
AlterAction          ::= ( 'ADD' | 'MODIFY' ) 'COLUMN'? ColumnDefinition
                       | 'RENAME' ( 'TO' Identifier | 'COLUMN' Identifier 'TO' Identifier )
                       | 'DROP' 'COLUMN' Identifier

DropTableStatement   ::= 'DROP' 'TABLE' Identifier

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

SelectStatement      ::= 'SELECT' SelectList 'FROM' Identifier WhereClause? LimitClause?
SelectList           ::= '*' | SelectColumn ( ',' SelectColumn )*
SelectColumn         ::= Expression ( 'AS' Identifier )?

InsertStatement      ::= 'INSERT' 'INTO' Identifier
                         ( '(' Identifier ( ',' Identifier )* ')' )?
                         'VALUES' ValueRow ( ',' ValueRow )*

ValueRow             ::= '(' Expression ( ',' Expression )* ')'

UpdateStatement      ::= 'UPDATE' Identifier 'SET' SetItem ( ',' SetItem )* WhereClause?
SetItem              ::= Identifier '=' Expression

DeleteStatement      ::= 'DELETE' 'FROM' Identifier WhereClause?

WhereClause          ::= 'WHERE' Condition
Condition            ::= OrCondition
OrCondition          ::= AndCondition ( 'OR' AndCondition )*
AndCondition         ::= NotCondition ( 'AND' NotCondition )*
NotCondition         ::= ConditionPrimary | 'NOT' NotCondition
ConditionPrimary     ::= Predicate | '(' Condition ')'
Predicate            ::= Expression ComparisonOperator Expression
ComparisonOperator   ::= '=' | '!=' | '<>' | '<' | '>' | '<=' | '>='

LimitClause          ::= 'LIMIT' IntegerLiteral

Expression           ::= Term ( ( '+' | '-' ) Term )*
Term                 ::= Factor ( ( '*' | '/' | '%' ) Factor )*
Factor               ::= Literal | Identifier | '(' Expression ')' | '-' Factor

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

### Notes

- **Identifier**: Governs database, table, and column names. They must begin with a letter and can include letters, digits, and underscores.
- **Literal**: Denotes fixed data values.
- **NullLiteral / BooleanLiteral**: Captures SQL boolean flags (`TRUE`/`FALSE`) and the missing data flag (`NULL`).
- **NumericLiteral / IntegerLiteral / FloatLiteral**: Governs integer and fractional digits.
- **StringLiteral**: Resolves single-quoted character sequences representing raw text values.
- **Letter / Digit / Character**: Fundamental character sets allowed within identifiers and string values.
