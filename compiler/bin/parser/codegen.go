package parser

import (
	"fmt"
	"strconv"
	"strings"
)

type CodeGenerator struct {
	builder strings.Builder
	graph   *ProgramGraph
	vars    map[*GraphNode]string
	serial  int
	indent  string
	proven  map[*GraphNode]bool
}

func NewCodeGenerator() *CodeGenerator { return &CodeGenerator{} }
func (cg *CodeGenerator) line(format string, args ...interface{}) {
	fmt.Fprintf(&cg.builder, cg.indent+format+"\n", args...)
}
func (cg *CodeGenerator) Generate(graph *ProgramGraph) (string, error) {
	return cg.generate(graph, nil)
}

// GenerateWithProvenChecks is a trusted compiler-internal lowering boundary.
// Decisions must come from fresh exact-site proofs, never user assertions or
// deserialized reports. Prefer optimization.Generate, which owns that gate.
// The graph must not be concurrently mutated during analysis/lowering.
func (cg *CodeGenerator) GenerateWithProvenChecks(graph *ProgramGraph, proven map[*GraphNode]bool) (string, error) {
	return cg.generate(graph, proven)
}
func (cg *CodeGenerator) generate(graph *ProgramGraph, proven map[*GraphNode]bool) (string, error) {
	cg.proven = proven
	defer func() { cg.proven = nil }()
	if graph == nil || !graph.Validated {
		return "", fmt.Errorf("code generation requires a validated semantic graph")
	}
	cg.builder.Reset()
	cg.graph = graph
	cg.serial = 0
	cg.indent = ""
	cg.builder.WriteString("#include <stdint.h>\n#include <inttypes.h>\n#include <stddef.h>\n#include <stdio.h>\n#include <stdlib.h>\n\n")
	cg.runtime()
	for _, node := range sortedFunctionNodes(graph) {
		f := graph.Functions[node.ID]
		cg.line("%s;", cg.signature(f))
	}
	for _, node := range sortedFunctionNodes(graph) {
		cg.function(graph.Functions[node.ID])
	}
	if f := graph.Functions["main"]; f != nil {
		cg.line("int main(void) { %s(); return 0; }", f.CName)
	}
	return cg.builder.String(), nil
}
func (cg *CodeGenerator) signature(f *FunctionInfo) string {
	params := []string{}
	for i, p := range f.Decl.Parameters {
		params = append(params, fmt.Sprintf("%s lk_p%d", cg.mapType(p.Type), i))
	}
	if len(params) == 0 {
		params = append(params, "void")
	}
	return fmt.Sprintf("%s %s(%s)", cg.mapType(f.Decl.ReturnType), f.CName, strings.Join(params, ", "))
}
func (cg *CodeGenerator) function(f *FunctionInfo) {
	cg.line("\n%s {", cg.signature(f))
	cg.indent = "    "
	cg.vars = map[*GraphNode]string{}
	for i, p := range f.Decl.Parameters {
		n := cg.graph.Nodes[f.Decl.Name+":"+p.Name]
		cg.vars[n] = fmt.Sprintf("lk_p%d", i)
		cg.line("(void)lk_p%d;", i)
	}
	cg.block(f.Decl.Body)
	cg.line("// Scope Exit: all local scalar storage expires here.")
	cg.indent = ""
	cg.line("}")
}
func (cg *CodeGenerator) block(block *BlockStatement) {
	count := 0
	for _, s := range block.Statements {
		if _, ok := s.(*LetStatement); ok {
			count++
		}
	}
	if count > 0 {
		// Typed union slots provide alignment and valid C effective types.
		// The exact slot count is known; references cannot escape this frame.
		cg.line("LeekSlot lk_buffer[%d];", count)
		cg.line("LeekArena lk_arena = { lk_buffer, 0 };")
	}
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *LetStatement:
			value := cg.expr(s.Expr)
			name := fmt.Sprintf("lk_v%d", cg.serial)
			cg.serial++
			cg.line("%s* %s = &arena_alloc(&lk_arena)->%s;", cg.mapType(s.Type), name, s.Type)
			cg.line("*%s = %s;", name, value)
			cg.vars[cg.graph.Declarations[s]] = "*" + name
		case *ReturnStatement:
			if s.Value == nil {
				cg.line("return;")
			} else {
				value := cg.expr(s.Value)
				cg.line("return %s;", value)
			}
		case *ExpressionStatement:
			cg.expr(s.Expr)
		case *IfStatement:
			condition := cg.expr(s.Condition)
			cg.line("if (%s) {", condition)
			cg.indent += "    "
			cg.block(s.Then)
			cg.indent = cg.indent[:len(cg.indent)-4]
			if s.Else != nil {
				cg.line("} else {")
				cg.indent += "    "
				cg.block(s.Else)
				cg.indent = cg.indent[:len(cg.indent)-4]
			}
			cg.line("}")
		}
	}
}
func (cg *CodeGenerator) temp(typ, rhs string) string {
	name := fmt.Sprintf("lk_t%d", cg.serial)
	cg.serial++
	cg.line("%s %s = %s;", cg.mapType(typ), name, rhs)
	cg.line("(void)%s;", name)
	return name
}
func location(e *Expr) string {
	return strconv.Quote(fmt.Sprintf("%s:%d:%d", e.Pos.Filename, e.Pos.Line, e.Pos.Column))
}
func (cg *CodeGenerator) expr(e *Expr) string {
	info := cg.graph.Expressions[e]
	if info.Constant != "" {
		return cg.temp(info.Type, cg.constant(info))
	}
	switch e.Kind {
	case "name":
		return cg.temp(info.Type, cg.vars[info.Binding])
	case "unary":
		a := cg.expr(e.Left)
		switch e.Name {
		case "!":
			return cg.temp(info.Type, "!"+a)
		case "+":
			return cg.temp(info.Type, a)
		default:
			return cg.temp(info.Type, fmt.Sprintf("lk_sub_%s(0, %s, %s)", info.Type, a, location(e)))
		}
	case "binary":
		a := cg.expr(e.Left)
		if e.Name == "&&" || e.Name == "||" {
			result := cg.temp("bool", a)
			cond := result
			if e.Name == "||" {
				cond = "!" + cond
			}
			cg.line("if (%s) {", cond)
			cg.indent += "    "
			b := cg.expr(e.Right)
			cg.line("%s = %s;", result, b)
			cg.indent = cg.indent[:len(cg.indent)-4]
			cg.line("}")
			return result
		}
		b := cg.expr(e.Right)
		op := map[string]string{"+": "add", "-": "sub", "*": "mul", "/": "div", "%": "rem"}[e.Name]
		if op != "" {
			if cg.proven[info.Node] && integerType(info.Type) && info.Type[0] == 'u' {
				cg.line("// arithmetic %s: proven guard elided", info.Node.ID)
				return cg.temp(info.Type, fmt.Sprintf("(%s)(%s %s %s)", cg.mapType(info.Type), a, e.Name, b))
			}
			cg.line("// arithmetic %s: checked", info.Node.ID)
			return cg.temp(info.Type, fmt.Sprintf("lk_%s_%s(%s, %s, %s)", op, info.Type, a, b, location(e)))
		}
		return cg.temp(info.Type, a+" "+e.Name+" "+b)
	case "call", "composition":
		values := []string{}
		if e.Kind == "composition" {
			values = append(values, cg.expr(e.Left))
		}
		for _, arg := range e.Args {
			values = append(values, cg.expr(arg))
		}
		if info.Callee.Builtin {
			arg := e.Args[0]
			typ := cg.graph.Expressions[arg].Type
			macro, cast := "PRIu64", "uint64_t"
			// Emit standard inttypes format macros, not platform-dependent C widths.
			if integerType(typ) && typ[0] == 'i' {
				macro = "PRId64"
				cast = "int64_t"
			}
			if typ == "f32" || typ == "f64" {
				precision := "%.17g"
				if typ == "f32" {
					precision = "%.9g"
				}
				cg.line("if (printf(\"%s\\n\", (double)%s) < 0 || fflush(stdout) == EOF) lk_fail(%s, \"output failure\");", precision, values[0], location(e))
			} else {
				cg.line("if (printf(\"%%\" %s \"\\n\", (%s)%s) < 0 || fflush(stdout) == EOF) lk_fail(%s, \"output failure\");", macro, cast, values[0], location(e))
			}
			return ""
		}
		call := info.Callee.CName + "(" + strings.Join(values, ", ") + ")"
		if info.Type == "" {
			cg.line("%s;", call)
			return ""
		}
		return cg.temp(info.Type, call)
	}
	panic("validated expression has no lowering")
}
func (cg *CodeGenerator) constant(i *ExprInfo) string {
	c := i.Constant
	if i.Type == "bool" {
		return c
	}
	if i.Type == "f32" || i.Type == "f64" {
		suffix := ""
		if i.Type == "f32" {
			suffix = "f"
		}
		return "(" + cg.mapType(i.Type) + ")(" + c + ".0" + suffix + ")"
	}
	if i.Type[0] == 'u' {
		return "(" + cg.mapType(i.Type) + ")UINT64_C(" + c + ")"
	}
	if c == "-9223372036854775808" {
		return "INT64_MIN"
	}
	if strings.HasPrefix(c, "-") {
		c = "(-INT64_C(" + c[1:] + "))"
	} else {
		c = "INT64_C(" + c + ")"
	}
	return "(" + cg.mapType(i.Type) + ")" + c
}
func (cg *CodeGenerator) mapType(t string) string {
	switch t {
	case "":
		return "void"
	case "bool":
		return "_Bool"
	case "f32":
		return "float"
	case "f64":
		return "double"
	}
	if t[0] == 'u' {
		return "uint" + t[1:] + "_t"
	}
	return "int" + t[1:] + "_t"
}
func (cg *CodeGenerator) runtime() {
	cg.builder.WriteString(`// Checked failure is a language outcome, not an arbitrary user I/O capability.
static _Noreturn void lk_fail(const char* location, const char* reason) {
    fprintf(stderr, "%s: runtime failure: %s\n", location, reason);
    exit(1);
}
typedef union {
    uint8_t u8; uint16_t u16; uint32_t u32; uint64_t u64;
    int8_t i8; int16_t i16; int32_t i32; int64_t i64;
    float f32; double f64; _Bool bool;
} LeekSlot;
typedef struct { LeekSlot* buffer; size_t offset; } LeekArena;
static inline LeekSlot* arena_alloc(LeekArena* arena) {
    return &arena->buffer[arena->offset++];
}
`)
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		ct := cg.mapType(typ)
		for _, op := range []string{"add", "sub", "mul"} {
			cg.line("static inline %s lk_%s_%s(%s a, %s b, const char* loc) {", ct, op, typ, ct, ct)
			cg.line("    %s result;", ct)
			cg.line("    if (__builtin_%s_overflow(a, b, &result)) lk_fail(loc, \"integer overflow or underflow\");", op)
			cg.line("    return result;\n}")
		}
		for _, op := range []string{"div", "rem"} {
			symbol := "/"
			if op == "rem" {
				symbol = "%"
			}
			cg.line("static inline %s lk_%s_%s(%s a, %s b, const char* loc) {", ct, op, typ, ct, ct)
			cg.line("    if (b == 0) lk_fail(loc, \"division by zero\");")
			if typ[0] == 'i' {
				cg.line("    if (a == INT%s_MIN && b == -1) lk_fail(loc, \"integer overflow or underflow\");", typ[1:])
			}
			cg.line("    return (%s)(a %s b);\n}", ct, symbol)
		}
	}
}
