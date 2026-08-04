package parser

import (
	"fmt"
	"strings"
	"text/scanner"
)

// ============================================================================
// 1. MATHEMATICAL CORE GRAPH SPECIFICATION (The Backend Truth)
// ============================================================================

type NodeType int

const (
	PureScope NodeType = iota
	ImpureScope
	ParallelScope
	MemoryScope
	IONode
)

type EdgeType int

const (
	DataEdge EdgeType = iota
	ControlEdge
	EffectEdge
	KpnFifoEdge
)

type KpnMetadata struct {
	DataType   string
	IsBlocking bool
}

type GraphEdge struct {
	Type        EdgeType
	From        *GraphNode
	To          *GraphNode
	KpnChannels *KpnMetadata
}

type GraphNode struct {
	ID         string
	Type       NodeType
	Incoming   []*GraphEdge
	Outgoing   []*GraphEdge
	Children   []*GraphNode
	Arena      *ArenaLayout
	SourceNode Node
}

type ArenaLayout struct {
	ScopeID string
}

type ProgramGraph struct {
	Nodes  map[string]*GraphNode
	Edges  []*GraphEdge
	Starts []*GraphNode
}

func NewProgramGraph() *ProgramGraph {
	return &ProgramGraph{
		Nodes:  make(map[string]*GraphNode),
		Edges:  make([]*GraphEdge, 0),
		Starts: make([]*GraphNode, 0),
	}
}

// ============================================================================
// 2. COMPILER VERIFICATION HELPERS
// ============================================================================

func (p *ProgramGraph) ToDOT() string {
	var sb strings.Builder
	sb.WriteString("digraph LeekProgramGraph {\n")
	sb.WriteString("    node [shape=box, style=\"filled,rounded\", fontname=\"Courier\", fontsize=11];\n")
	sb.WriteString("    edge [fontname=\"Courier\", fontsize=9];\n\n")

	for id, node := range p.Nodes {
		fillcolor := "#E1F5FE"
		if node.Type == ImpureScope {
			fillcolor = "#FFEBEE"
		}
		sb.WriteString(fmt.Sprintf("    \"%s\" [label=\"%s\", fillcolor=\"%s\"];\n", id, id, fillcolor))
	}
	sb.WriteString("\n")

	for _, edge := range p.Edges {
		var attrs []string
		switch edge.Type {
		case ControlEdge:
			attrs = append(attrs, "color=\"#1565C0\"", "style=\"dashed\"", "label=\"CONTROL\"")
		case DataEdge:
			attrs = append(attrs, "color=\"#2E7D32\"", "style=\"solid\"", "arrowhead=\"normal\"", "label=\"DATA/VALUE\"")
		case EffectEdge:
			attrs = append(attrs, "color=\"#C62828\"", "style=\"bold\"", "label=\"EFFECT\"")
		}

		sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\" [%s];\n",
			edge.From.ID, edge.To.ID, strings.Join(attrs, ", ")))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func getChildNodes(container *GraphNode) []*GraphNode {
	return container.Children
}

func containsNode(list []*GraphNode, target *GraphNode) bool {
	for _, node := range list {
		if node == target {
			return true
		}
	}
	return false
}

func VerifyKpnCompatibility(p *ProgramGraph, parallelScope *GraphNode) error {
	childNodes := getChildNodes(parallelScope)

	for _, node := range childNodes {
		if node.Type != PureScope {
			return fmt.Errorf("KPN Violation: Node %s inside parallel scope must be PURE", node.ID)
		}

		for _, edge := range node.Outgoing {
			if containsNode(childNodes, edge.To) {
				if edge.Type != KpnFifoEdge {
					return fmt.Errorf("KPN Violation: Illegal data path from %s to %s. Inter-process communication must strictly use KpnFifoEdge", edge.From.ID, edge.To.ID)
				}
				if edge.KpnChannels != nil && !edge.KpnChannels.IsBlocking {
					return fmt.Errorf("KPN Violation: FIFO channel from %s to %s must be blocking on read to guarantee determinism", edge.From.ID, edge.To.ID)
				}
			}
		}
	}
	return nil
}

// ============================================================================
// 3. AST FRONTEND NODE DEFINITIONS
// ============================================================================

type Parameter struct {
	Name string
	Type string
}

type Node interface {
	String() string
}

// SourcePos is retained on frontend nodes so later stages can report useful
// diagnostics without having to reconstruct positions from generated graph IDs.
type SourcePos = scanner.Position

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

type BlockStatement struct {
	Statements []Statement
}

func (bs *BlockStatement) String() string {
	var sb strings.Builder
	for _, stmt := range bs.Statements {
		sb.WriteString(stmt.String() + " ")
	}
	return fmt.Sprintf("{ %s}", sb.String())
}
func (bs *BlockStatement) statementNode() {}

type FunctionStatement struct {
	Name       string
	IsImpure   bool
	Parameters []Parameter
	Body       *BlockStatement
	Pos        SourcePos
}

func (fs *FunctionStatement) statementNode() {}
func (fs *FunctionStatement) String() string {
	prefix := "FN"
	if fs.IsImpure {
		prefix = "IMPURE_FN"
	}
	return fmt.Sprintf("(%s %s %s)", prefix, fs.Name, fs.Body.String())
}

type LetStatement struct {
	Name  string
	Type  string
	Value string
	Pos   SourcePos
}

func (ls *LetStatement) statementNode() {}
func (ls *LetStatement) String() string {
	return fmt.Sprintf("(LET %s %s = %s)", ls.Name, ls.Type, ls.Value)
}
