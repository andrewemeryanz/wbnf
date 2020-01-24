package wbnf

var grammarGrammarSrc = unfakeBackquote(`
// Non-terminals
grammar -> stmt+;
stmt    -> COMMENT | prod;
prod    -> IDENT "->" term+ ";";
term    -> @:op="^"
         ^ @:op="|"
         ^ @+
         ^ named quant*;
named   -> (IDENT op="=")? atom;
quant   -> op=/{[?*+]}
         | "{" min=INT? "," max=INT? "}"
         | op=/{<:|:>?} opt_leading=","? named opt_trailing=","?;
atom    -> IDENT | STR | RE | REF | "(" term ")" | "(" ")";

// Terminals
IDENT   -> /{@|[A-Za-z_\.]\w*};
STR     -> /{ " (?: \\. | [^\\"] )* "
            | ' (?: \\. | [^\\'] )* '
            | ‵ (?: ‵‵  | [^‵]   )* ‵
            };
INT     -> /{\d+};
RE      -> /{
             /{
               ((?:
                 \\.
                 | { (?: (?: \d+(?:,\d*)? | ,\d+ ) \} )?
                 | \[ (?: \\] | [^\]] )+ ]
                 | [^\\{\}]
               )*)
             \}
           };
REF		-> "\\" IDENT;
COMMENT -> /{ //.*$
            | (?s: /\* (?: [^*] | \*+[^*/] ) \*/ )
            };

// Special
.wrapRE -> /{\s*()\s*};
`)