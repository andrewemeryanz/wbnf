package bootstrap

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/arr-ai/wbnf/parser"
)

var (
	grammarR = Rule("grammar")
	stmt     = Rule("stmt")
	prod     = Rule("prod")
	term     = Rule("term")
	named    = Rule("named")
	atom     = Rule("atom")
	quant    = Rule("quant")
	ident    = Rule("IDENT")
	str      = Rule("STR")
	intR     = Rule("INT")
	re       = Rule("RE")
	comment  = Rule("COMMENT")

	// WrapRE is a special rule to indicate a wrapper around all regexps and
	// strings. When supplied in the form "pre()post", then all regexes will be
	// wrapped in "pre(?:" and ")post" and all strings will be escaped using
	// regexp.QuoteMeta then likewise wrapped.
	WrapRE = Rule(".wrapRE")
)

var RootRule = grammarR

// unfakeBackquote replaces reversed prime with grave accent (backquote) in
// order to make the grammar below more readable.
func unfakeBackquote(s string) string {
	return strings.ReplaceAll(s, "‵", "`")
}

var grammarGrammarSrc = unfakeBackquote(`
// Non-terminals
grammar -> stmt+;
stmt    -> COMMENT | prod;
prod    -> IDENT "->" term+ ";";
term    -> term:op="^"
         ^ term:op="|"
         ^ term+
         ^ named quant*;
named   -> (IDENT op="=")? atom;
quant   -> op=/{[?*+]}
         | "{" min=INT? "," max=INT? "}"
         | op=/{<:|:>?} opt_leading=","? named opt_trailing=","?;
atom    -> IDENT | STR | RE | "(" term ")" | "(" ")";

// Terminals
IDENT   -> /{[A-Za-z_\.]\w*};
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
COMMENT -> /{ //.*$
            | (?s: /\* (?: [^*] | \*+[^*/] ) \*/ )
            };

// Special
.wrapRE -> /{\s*()\s*};
`)

var grammarGrammar = Grammar{
	// Non-terminals
	grammarR: Some(stmt),
	stmt:     Oneof{comment, prod},
	prod:     Seq{ident, S("->"), Some(term), S(";")},
	term: Stack{
		Delim{Term: term, Sep: Eq("op", S("^"))},
		Delim{Term: term, Sep: Eq("op", S("|"))},
		Some(term),
		Seq{named, Any(quant)},
	},
	quant: Oneof{
		Eq("op", RE(`[?*+]`)),
		Seq{S("{"), Opt(Eq("min", intR)), S(","), Opt(Eq("max", intR)), S("}")},
		Seq{
			Eq("op", RE(`<:|:>?`)),
			Opt(Eq("opt_leading", S(","))),
			named,
			Opt(Eq("opt_trailing", S(","))),
		},
	},
	named: Seq{Opt(Seq{ident, Eq("op", S("="))}), atom},
	atom:  Oneof{ident, str, re, Seq{S("("), term, S(")")}, Seq{S("("), S(")")}},

	// Terminals
	ident:   RE(`[A-Za-z_\.]\w*`),
	str:     RE(unfakeBackquote(`"(?:\\.|[^\\"])*"|'(?:\\.|[^\\'])*'|‵(?:‵‵|[^‵])*‵`)),
	intR:    RE(`\d+`),
	re:      RE(`/{((?:\\.|{(?:(?:\d+(?:,\d*)?|,\d+)\})?|\[(?:\\]|[^\]])+]|[^\\{\}])*)\}`),
	comment: RE(`//.*$|(?s:/\*(?:[^*]|\*+[^*/])\*/)`),

	// Special
	WrapRE: RE(`\s*()\s*`),
}

func NodeRule(v interface{}) Rule {
	return Rule(v.(parser.Node).Tag)
}

type Grammar map[Rule]Term

// Build the grammar grammar from grammarGrammarSrc and check that it matches
// grammarGrammar.
var core = func() Parsers {
	parsers := grammarGrammar.Compile()

	r := parser.NewScanner(grammarGrammarSrc)
	v, err := parsers.Parse(grammarR, r)
	if err != nil {
		panic(err)
	}
	if err := parsers.Grammar().ValidateParse(v); err != nil {
		panic(err)
	}
	g := v.(parser.Node)

	newGrammarGrammar := NewFromNode(g)

	if diff := DiffGrammars(grammarGrammar, newGrammarGrammar); !diff.Equal() {
		panic(fmt.Errorf(
			"mismatch between parsed and hand-crafted core grammar"+
				"\nold: %v"+
				"\nnew: %v"+
				"\ndiff: %#v",
			grammarGrammar, newGrammarGrammar, diff,
		))
	}

	return newGrammarGrammar.Compile()
}()

func Core() Parsers {
	return core
}

// ValidateParse performs numerous checks on a generated AST to ensure it
// conforms to the parser that generated it. It is useful for testing the
// parser engine, but also for any tools that synthesise parser output.
func (g Grammar) ValidateParse(v interface{}) error {
	rule := NodeRule(v)
	return g[rule].ValidateParse(g, rule, v)
}

// Unparse inverts the action of a parser, taking a generated AST and producing
// the source it came from. Currently, it doesn't quite do that, and is only
// being used for quick eyeballing to validate output.
func (g Grammar) Unparse(v interface{}, w io.Writer) (n int, err error) {
	rule := NodeRule(v)
	return g[rule].Unparse(g, v, w)
}

// Parsers holds Parsers generated by Grammar.Compile.
type Parsers struct {
	parsers    map[Rule]parser.Parser
	grammar    Grammar
	singletons PathSet
}

func (p Parsers) Grammar() Grammar {
	return p.grammar
}

func (p Parsers) ValidateParse(v interface{}) error {
	return p.grammar.ValidateParse(v)
}

func (p Parsers) Unparse(v interface{}, w io.Writer) (n int, err error) {
	return p.grammar.Unparse(v, w)
}

// Parse parses some source per a given rule.
func (p Parsers) Parse(rule Rule, input *parser.Scanner) (interface{}, error) {
	start := *input
	for {
		var v interface{}
		if err := p.parsers[rule].Parse(input, &v); err != nil {
			return nil, err
		}

		if input.String() == "" {
			return v, nil
		}

		if input.Offset() == start.Offset() {
			return nil, fmt.Errorf("unconsumed input: %v", input.Context())
		}
	}
}

// MustParse calls Parse and returns the result or panics if an error was
// returned.
func (p Parsers) MustParse(rule Rule, input *parser.Scanner) interface{} {
	i, err := p.Parse(rule, input)
	if err != nil {
		panic(err)
	}
	return i
}

// Singletons returns the set of names that will produce exactly one child
// under a given production.
func (p Parsers) Singletons() PathSet {
	return p.singletons
}

// Term represents the terms of a grammar specification.
type Term interface {
	fmt.Stringer
	Parser(name Rule, c cache) parser.Parser
	ValidateParse(g Grammar, rule Rule, v interface{}) error
	Unparse(g Grammar, v interface{}, w io.Writer) (n int, err error)
	Resolve(oldRule, newRule Rule) Term
}

type Associativity int

func NewAssociativity(s string) Associativity {
	switch s {
	case ":":
		return NonAssociative
	case ":>":
		return LeftToRight
	case "<:":
		return RightToLeft
	}
	panic(BadInput)
}

func (a Associativity) String() string {
	switch {
	case a < 0:
		return "<:"
	case a > 0:
		return ":>"
	}
	return ":"
}

const (
	RightToLeft Associativity = iota - 1
	NonAssociative
	LeftToRight
)

type (
	Rule  string
	S     string
	RE    string
	Seq   []Term
	Oneof []Term
	Stack []Term
	Delim struct {
		Term            Term
		Sep             Term
		Assoc           Associativity
		CanStartWithSep bool
		CanEndWithSep   bool
	}
	Quant struct {
		Term Term
		Min  int
		Max  int // 0 = infinity
	}
	Named struct {
		Name string
		Term Term
	}
)

func NonAssoc(term, sep Term) Delim { return Delim{Term: term, Sep: sep, Assoc: NonAssociative} }
func L2R(term, sep Term) Delim      { return Delim{Term: term, Sep: sep, Assoc: LeftToRight} }
func R2L(term, sep Term) Delim      { return Delim{Term: term, Sep: sep, Assoc: RightToLeft} }

func Opt(term Term) Quant  { return Quant{Term: term, Max: 1} }
func Any(term Term) Quant  { return Quant{Term: term} }
func Some(term Term) Quant { return Quant{Term: term, Min: 1} }

func Eq(name string, term Term) Named {
	return Named{Name: name, Term: term}
}

func join(terms []Term, sep string) string {
	s := []string{}
	for _, t := range terms {
		s = append(s, t.String())
	}
	return strings.Join(s, sep)
}

func (t Quant) Contains(i int) bool {
	return t.Min <= i && (t.Max == 0 || i <= t.Max)
}

func (t Quant) counter() counter {
	max := t.Max
	if max == 0 {
		max = 2
	}
	return newCounter(t.Min, max)
}

func (g Grammar) String() string {
	keys := make([]string, 0, len(g))
	for key := range g {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)

	var sb strings.Builder
	count := 0
	for _, key := range keys {
		if count > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "%s -> %v", key, g[Rule(key)])
		count++
	}
	return sb.String()
}

func (t Rule) String() string  { return string(t) }
func (t S) String() string     { return fmt.Sprintf("%q", string(t)) }
func (t RE) String() string    { return fmt.Sprintf("/%v/", string(t)) }
func (t Seq) String() string   { return join(t, " ") }
func (t Oneof) String() string { return join(t, " | ") }
func (t Stack) String() string { return join(t, " ^ ") }
func (t Delim) String() string { return fmt.Sprintf("%v%s%v", t.Term, t.Assoc, t.Sep) }
func (t Named) String() string { return fmt.Sprintf("%s=%v", t.Name, t.Term) }
func (t Quant) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%v", t.Term)
	switch [2]int{t.Min, t.Max} {
	case [2]int{0, 0}:
		sb.WriteString("*")
	case [2]int{0, 1}:
		sb.WriteString("?")
	case [2]int{1, 0}:
		sb.WriteString("+")
	case [2]int{1, 1}:
		panic(Inconceivable)
	default:
		sb.WriteString("{")
		if t.Min != 0 {
			fmt.Fprintf(&sb, "%d", t.Min)
		}
		sb.WriteString(",")
		if t.Max != 0 {
			fmt.Fprintf(&sb, "%d", t.Max)
		}
		sb.WriteString("}")
	}
	return sb.String()
}
