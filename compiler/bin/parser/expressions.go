package parser

import "strings"

// Expr retains composition as a distinct operation, not rewritten call text.
// For composition, Left is the source and Args are the additional arguments.
// For calls, Name identifies a direct callee. No indirect calls exist yet.
type Expr struct {
	Kind        string // literal, name, unary, binary, call, composition
	Name        string // identifier, literal spelling, or operator
	Left, Right *Expr
	Args        []*Expr
	Pos         SourcePos
}

func (*Expr) expressionNode() {}
func (e *Expr) String() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case "unary":
		return "(" + e.Name + e.Left.String() + ")"
	case "binary":
		return "(" + e.Left.String() + " " + e.Name + " " + e.Right.String() + ")"
	case "call", "composition":
		args := []string{}
		for _, a := range e.Args {
			args = append(args, a.String())
		}
		call := e.Name + "(" + strings.Join(args, ", ") + ")"
		if e.Kind == "composition" {
			return "(" + e.Left.String() + " -> " + call + ")"
		}
		return call
	default:
		return e.Name
	}
}

// IfStatement is conditional control flow, not a speculative expression.
// Each branch is a lexical scope; values leave it only through return.
type IfStatement struct {
	Condition  *Expr
	Then, Else *BlockStatement
	Pos        SourcePos
}

func (*IfStatement) statementNode() {}
func (s *IfStatement) String() string {
	out := "if " + s.Condition.String() + " " + s.Then.String()
	if s.Else != nil {
		out += " else " + s.Else.String()
	}
	return out
}

type ReturnStatement struct {
	Value *Expr
	Pos   SourcePos
}

func (*ReturnStatement) statementNode()   {}
func (s *ReturnStatement) String() string { return "return " + s.Value.String() }

type ExpressionStatement struct {
	Expr *Expr
	Pos  SourcePos
}

func (*ExpressionStatement) statementNode()   {}
func (s *ExpressionStatement) String() string { return s.Expr.String() }

// Effects and failure are independent: a checked arithmetic failure does not
// grant permission to perform I/O, nor does purity grant permission to erase it.
type EffectSet uint8

const (
	EffectNone     EffectSet = 0
	EffectIO       EffectSet = 1
	EffectBoundary EffectSet = 2 // Explicit impure declarations remain effectful contracts.
)

type ExprInfo struct {
	Dataflow     *DataflowOp   // semantic sequence operation, not an opaque ordinary call
	Reference    *FunctionInfo // static callback reference; never a runtime function value
	Type         string
	Effects      EffectSet // Whole expression, including source and extra arguments.
	StageEffects EffectSet // Composition callee only; always none after validation.
	MayFail      bool
	Constant     string // Validated literal, never unchecked source text for C.
	Callee       *FunctionInfo
	Binding      *GraphNode
	Node         *GraphNode
}
type CallSite struct {
	Expr   *Expr
	Callee *FunctionInfo
}
type FunctionInfo struct {
	Decl    *FunctionStatement
	Node    *GraphNode
	Parent  *FunctionInfo
	Calls   []CallSite
	Effects EffectSet
	MayFail bool
	CName   string
	Builtin bool
}
