package wbnf

import (
	"fmt"
	"testing"

	"github.com/arr-ai/wbnf/parser"
	"github.com/stretchr/testify/assert"
)

func TestParserNodeToNode(t *testing.T) {
	p := Core()
	v := p.MustParse("grammar", parser.NewScanner(`expr -> @:op="+" > @:op="*" > \d+;`)).(parser.Node)
	g := p.Grammar()
	n := FromParserNode(g, v)
	u := ToParserNode(g, n).(parser.Node)
	parser.AssertEqualNodes(t, v, u)

	p = NewFromNode(v).Compile(&v)
	v = p.MustParse(parser.Rule("expr"), parser.NewScanner(`1+2*3`)).(parser.Node)
	g = p.Grammar()
	n = FromParserNode(g, v)
	u = ToParserNode(g, n).(parser.Node)
	parser.AssertEqualNodes(t, v, u)
}

func TestTinyXMLGrammar(t *testing.T) {
	t.Parallel()

	v, err := Core().Parse("grammar", parser.NewScanner(`
		xml  -> s "<" s NAME attr* s ">" xml* "</" s NAME s ">" | CDATA=[^<]+;
		attr -> s NAME s "=" s value=/{"[^"]*"};
		NAME -> [A-Za-z_:][-A-Za-z0-9._:]*;
		s    -> \s*;
	`))
	assert.NoError(t, err)

	node := v.(parser.Node)
	xmlParser := NewFromNode(node).Compile(&node)

	src := parser.NewScanner(`<a x="1">hello <b>world!</b></a>`)
	orig := *src
	s := func(offset int, expected string) Leaf {
		end := offset + len(expected)
		slice := orig.Slice(offset, end).String()
		if slice != expected {
			panic(fmt.Errorf("expecting %q, got %q", expected, slice))
		}
		return Leaf(*orig.Slice(offset, end))
	}

	xml, err := xmlParser.Parse(parser.Rule("xml"), src)
	assert.NoError(t, err)

	a := FromParserNode(xmlParser.Grammar(), xml)

	assert.EqualValues(t,
		Branch{
			RuleTag:   One{Extra{parser.Rule("xml")}},
			ChoiceTag: Many{Extra{0}},
			"":        Many{s(0, `<`), s(8, `>`), s(28, `</`), s(31, `>`)},
			"s":       Many{s(0, ``), s(1, ``), s(8, ``), s(30, ``), s(31, ``)},
			"NAME":    Many{s(1, `a`), s(30, `a`)},
			"attr": Many{Branch{
				"":      One{s(4, `=`)},
				"NAME":  One{s(3, `x`)},
				"s":     Many{s(2, ` `), s(4, ``), s(5, ``)},
				"value": One{s(5, `"1"`)},
			}},
			"xml": Many{
				Branch{
					ChoiceTag: Many{Extra{1}},
					"CDATA":   One{s(9, `hello `)},
				},
				Branch{
					ChoiceTag: Many{Extra{0}},
					"":        Many{s(15, `<`), s(17, `>`), s(24, `</`), s(27, `>`)},
					"s":       Many{s(15, ``), s(16, ``), s(17, ``), s(26, ``), s(27, ``)},
					"NAME":    Many{s(16, `b`), s(26, `b`)},
					"xml": Many{
						Branch{
							ChoiceTag: Many{Extra{1}},
							"CDATA":   One{s(18, `world!`)},
						},
					},
				},
			},
		},
		a,
	)
}
