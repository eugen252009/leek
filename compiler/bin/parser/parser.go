package parser

import (
	"fmt"
	"io"
	"text/scanner"
)

type token struct {
	kind rune
	text string
	pos  SourcePos
}
type Parser struct {
	tokens []token
	index  int
	lexErr error
}

func NewParser(src io.Reader, filename string) *Parser {
	p := &Parser{}
	var s scanner.Scanner
	s.Init(src)
	s.Filename = filename
	s.Mode = scanner.ScanIdents | scanner.ScanComments | scanner.SkipComments
	s.Error = func(s *scanner.Scanner, msg string) {
		if p.lexErr == nil {
			p.lexErr = fmt.Errorf("[%s] syntax error: %s", s.Position, msg)
		}
	}
	for {
		k := s.Scan()
		t := token{k, s.TokenText(), s.Position}
		// Decimal digits are collected here so leading zeroes never imply octal.
		if k >= '0' && k <= '9' {
			t.kind = scanner.Int
		}
		// Combine only adjacent operator characters, never across whitespace/comments.
		if len(p.tokens) > 0 {
			prev := &p.tokens[len(p.tokens)-1]
			joined := prev.text + t.text
			if prev.pos.Offset+len(prev.text) == t.pos.Offset {
				if prev.kind == scanner.Int && t.kind == scanner.Int {
					prev.text = joined
					continue
				}
				if len(prev.text) == 1 && len(t.text) == 1 {
					switch joined {
					case "->", "==", "!=", "<=", ">=", "&&", "||":
						prev.text = joined
						continue
					}
				}
			}
		}
		p.tokens = append(p.tokens, t)
		if k == scanner.EOF {
			break
		}
	}
	return p
}
func (p *Parser) current() token { return p.tokens[p.index] }
func (p *Parser) next() {
	if p.current().kind != scanner.EOF {
		p.index++
	}
}
func (p *Parser) take(s string) bool {
	if p.current().text == s {
		p.next()
		return true
	}
	return false
}
func (p *Parser) expect(s string) error {
	if !p.take(s) {
		return p.error("expected " + s)
	}
	return nil
}
func (p *Parser) identifier() (token, error) {
	t := p.current()
	if t.kind != scanner.Ident {
		return t, p.error("expected identifier")
	}
	switch t.text {
	case "fn", "impure", "let", "return", "true", "false", "if", "else":
		return t, p.error("reserved word cannot be an identifier")
	}
	p.next()
	return t, nil
}
func (p *Parser) ParseProgram() ([]Statement, error) {
	if p.lexErr != nil {
		return nil, p.lexErr
	}
	var out []Statement
	for p.current().kind != scanner.EOF {
		s, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
func (p *Parser) parseStatement() (Statement, error) {
	t := p.current()
	switch t.text {
	case "fn":
		return p.parseFunction(false)
	case "impure":
		p.next()
		if p.current().text != "fn" {
			return nil, p.error("expected 'fn' after 'impure'")
		}
		return p.parseFunction(true)
	case "if":
		p.next()
		condition, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		then, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		var otherwise *BlockStatement
		if p.take("else") {
			otherwise, err = p.parseBlock()
			if err != nil {
				return nil, err
			}
		}
		return &IfStatement{Condition: condition, Then: then, Else: otherwise, Pos: t.pos}, nil
	case "let":
		p.next()
		name, err := p.identifier()
		if err != nil {
			return nil, err
		}
		typ, err := p.identifier()
		if err != nil {
			return nil, err
		}
		if err = p.expect("="); err != nil {
			return nil, err
		}
		value, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		return &LetStatement{Name: name.text, Type: typ.text, Expr: value, Value: value.String(), Pos: t.pos}, nil
	case "return":
		p.next()
		// A bare return ends at a newline or closing brace. Expressions following
		// return start on that line, but may continue over subsequent lines.
		if p.current().text == "}" || p.current().pos.Line > t.pos.Line {
			return &ReturnStatement{Pos: t.pos}, nil
		}
		e, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		return &ReturnStatement{Value: e, Pos: t.pos}, nil
	default:
		e, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		if e.Kind != "call" && e.Kind != "composition" {
			return nil, p.error("only calls or compositions may be expression statements")
		}
		return &ExpressionStatement{Expr: e, Pos: t.pos}, nil
	}
}
func (p *Parser) parseFunction(impure bool) (Statement, error) {
	pos := p.current().pos
	p.next()
	name, err := p.identifier()
	if err != nil {
		return nil, err
	}
	if err = p.expect("("); err != nil {
		return nil, err
	}
	var params []Parameter
	for p.current().text != ")" {
		n, err := p.identifier()
		if err != nil {
			return nil, err
		}
		typ, err := p.identifier()
		if err != nil {
			return nil, err
		}
		params = append(params, Parameter{Name: n.text, Type: typ.text, Pos: n.pos})
		if !p.take(",") && p.current().text != ")" {
			return nil, p.error("expected ',' or ')' after parameter")
		}
	}
	p.next()
	result := ""
	if p.current().kind == scanner.Ident {
		typ, err := p.identifier()
		if err != nil {
			return nil, err
		}
		result = typ.text
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &FunctionStatement{Name: name.text, IsImpure: impure, ReturnType: result, Parameters: params, Body: body, Pos: pos}, nil
}
func (p *Parser) parseBlock() (*BlockStatement, error) {
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	body := &BlockStatement{}
	for p.current().text != "}" && p.current().kind != scanner.EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body.Statements = append(body.Statements, stmt)
	}
	if err := p.expect("}"); err != nil {
		return nil, err
	}
	return body, nil
}
func precedence(op string) int {
	switch op {
	case "->":
		return 1
	case "||":
		return 2
	case "&&":
		return 3
	case "==", "!=":
		return 4
	case "<", "<=", ">", ">=":
		return 5
	case "+", "-":
		return 6
	case "*", "/", "%":
		return 7
	}
	return 0
}
func (p *Parser) expression(min int) (*Expr, error) {
	left, err := p.prefix()
	if err != nil {
		return nil, err
	}
	for {
		op := p.current()
		prec := precedence(op.text)
		if prec < min {
			break
		}
		p.next()
		if op.text == "->" {
			name, err := p.identifier()
			if err != nil {
				return nil, err
			}
			var args []*Expr
			if p.current().text == "(" {
				args, err = p.arguments()
				if err != nil {
					return nil, err
				}
			}
			if precedence(p.current().text) > 1 {
				return nil, p.error("composition stage must be a function name or application; parenthesize the composition before applying another operator")
			}
			left = &Expr{Kind: "composition", Name: name.text, Left: left, Args: args, Pos: op.pos}
		} else {
			right, err := p.expression(prec + 1)
			if err != nil {
				return nil, err
			}
			left = &Expr{Kind: "binary", Name: op.text, Left: left, Right: right, Pos: op.pos}
		}
	}
	return left, nil
}
func (p *Parser) prefix() (*Expr, error) {
	t := p.current()
	if t.text == "-" || t.text == "+" || t.text == "!" {
		p.next()
		e, err := p.prefix()
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: "unary", Name: t.text, Left: e, Pos: t.pos}, nil
	}
	if p.take("(") {
		e, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		if err = p.expect(")"); err != nil {
			return nil, err
		}
		return e, nil
	}
	if t.kind == scanner.Int || t.text == "true" || t.text == "false" {
		p.next()
		return &Expr{Kind: "literal", Name: t.text, Pos: t.pos}, nil
	}
	if t.kind == scanner.Ident {
		n, err := p.identifier()
		if err != nil {
			return nil, err
		}
		e := &Expr{Kind: "name", Name: n.text, Pos: n.pos}
		if p.current().text == "(" {
			e.Kind = "call"
			e.Args, err = p.arguments()
		}
		return e, err
	}
	return nil, p.error("expected expression")
}
func (p *Parser) arguments() ([]*Expr, error) {
	p.next()
	var args []*Expr
	for p.current().text != ")" {
		e, err := p.expression(1)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
		if !p.take(",") && p.current().text != ")" {
			return nil, p.error("expected ',' or ')' after argument")
		}
	}
	p.next()
	return args, nil
}
func (p *Parser) error(msg string) error {
	t := p.current()
	return fmt.Errorf("[%s:%d:%d] syntax error: %s (found %q)", t.pos.Filename, t.pos.Line, t.pos.Column, msg, t.text)
}
