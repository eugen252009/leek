package parser

import (
	"fmt"
	"io"
	"text/scanner"
)

type Parser struct {
	scanner scanner.Scanner
	ch      rune 
}

func NewParser(src io.Reader, filename string) *Parser {
	p := &Parser{}
	p.scanner.Init(src)
	p.scanner.Filename = filename
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanChars | scanner.ScanInts
	p.next()
	return p
}

func (p *Parser) next() {
	p.ch = p.scanner.Scan()
}

func (p *Parser) ParseProgram() ([]Statement, error) {
	var statements []Statement

	for p.ch != scanner.EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			statements = append(statements, stmt)
		}
	}

	return statements, nil
}

func (p *Parser) parseStatement() (Statement, error) {
	switch p.ch {
	case scanner.Ident:
		text := p.scanner.TokenText()

		switch text {
		case "fn":
			return p.parseFunction(false)

		case "impure":
			p.next()
			if p.ch == scanner.Ident && p.scanner.TokenText() == "fn" {
				return p.parseFunction(true)
			}
			return nil, p.error("expected 'fn' after 'impure'")

		case "let":
			return p.parseLetStatement()

		default:
			return nil, p.error(fmt.Sprintf("unexpected identifier %q", text))
		}

	default:
		return nil, p.error(fmt.Sprintf("unexpected token %q", p.scanner.TokenText()))
	}
}

func (p *Parser) parseFunction(isImpure bool) (Statement, error) {
	p.next() 

	if p.ch != scanner.Ident {
		return nil, p.error("expected function name")
	}
	fnName := p.scanner.TokenText()
	p.next()

	if p.ch != '(' {
		return nil, p.error("expected '(' after function name")
	}
	p.next()

	params, err := p.parseParameters()
	if err != nil {
		return nil, err
	}

	if p.ch != ')' {
		return nil, p.error("expected ')' after parameters")
	}
	p.next()

	if p.ch != '{' {
		return nil, p.error("expected '{' to start function body")
	}

	body, err := p.parseBlockStatement()
	if err != nil {
		return nil, err
	}

	return &FunctionStatement{
		Name:       fnName,
		IsImpure:   isImpure,
		Parameters: params,
		Body:       body,
	}, nil
}

func (p *Parser) parseParameters() ([]Parameter, error) {
	var params []Parameter

	for p.ch != ')' && p.ch != scanner.EOF {
		if p.ch != scanner.Ident {
			return nil, p.error("expected parameter name")
		}
		paramName := p.scanner.TokenText()
		p.next()

		if p.ch != scanner.Ident {
			return nil, p.error(fmt.Sprintf("expected type for parameter %s", paramName))
		}
		paramType := p.scanner.TokenText()
		p.next()

		params = append(params, Parameter{Name: paramName, Type: paramType})

		if p.ch == ',' {
			p.next()
		}
	}

	return params, nil
}

func (p *Parser) parseBlockStatement() (*BlockStatement, error) {
	p.next() 
	block := &BlockStatement{Statements: []Statement{}}

	for p.ch != '}' && p.ch != scanner.EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
	}

	if p.ch != '}' {
		return nil, p.error("expected '}' to close block")
	}
	p.next()

	return block, nil
}

func (p *Parser) parseLetStatement() (Statement, error) {
	p.next()

	if p.ch != scanner.Ident {
		return nil, p.error("expected variable name after 'let'")
	}
	varName := p.scanner.TokenText()
	p.next()

	if p.ch != scanner.Ident {
		return nil, p.error(fmt.Sprintf("expected type for variable %s", varName))
	}
	varType := p.scanner.TokenText()
	p.next()

	if p.ch != '=' {
		return nil, p.error("expected '=' in variable declaration")
	}
	p.next()

	if p.ch != scanner.Ident && p.ch != scanner.Int {
		return nil, p.error("expected value after '='")
	}
	val := p.scanner.TokenText()
	p.next()

	return &LetStatement{
		Name:  varName,
		Type:  varType,
		Value: val,
	}, nil
}

func (p *Parser) error(msg string) error {
	pos := p.scanner.Position
	return fmt.Errorf("[%s:%d:%d] syntax error: %s (found %q)", pos.Filename, pos.Line, pos.Column, msg, p.scanner.TokenText())
}
