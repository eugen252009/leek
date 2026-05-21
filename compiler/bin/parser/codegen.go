package parser

import (
	"fmt"
	"strings"
)

type CodeGenerator struct {
	builder strings.Builder
}

func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{}
}

func (cg *CodeGenerator) Generate(graph *ProgramGraph) (string, error) {
	cg.builder.WriteString("#include <stdio.h>\n")
	cg.builder.WriteString("#include <stdint.h>\n")
	cg.builder.WriteString("#include <stdlib.h>\n\n")

	cg.generateArenaRuntime()

	cg.builder.WriteString("// --- Function Prototypes ---\n")
	for _, node := range graph.Nodes {
		if _, isFn := node.SourceNode.(*FunctionStatement); isFn {
			cg.generateFunctionPrototype(node)
		}
	}
	cg.builder.WriteString("\n")

	cg.builder.WriteString("// --- Function Implementations ---\n")
	for _, node := range graph.Nodes {
		if _, isFn := node.SourceNode.(*FunctionStatement); isFn {
			if err := cg.generateFunctionBody(node); err != nil {
				return "", err
			}
		}
	}

	return cg.builder.String(), nil
}

func (cg *CodeGenerator) generateArenaRuntime() {
	cg.builder.WriteString(`// Minimal linear Arena Runtime (A6 — Linear Memory Binding)
typedef struct {
    uint8_t* buffer;
    size_t size;
    size_t offset;
} LeekArena;

static inline void* arena_alloc(LeekArena* arena, size_t size) {
    // 8-Byte Alignment sicherstellen
    size_t aligned_size = (size + 7) & ~7;
    if (arena->offset + aligned_size > arena->size) {
        fprintf(stderr, "Out of Memory in Arena Scope!\n");
        exit(1);
    }
    void* ptr = &arena->buffer[arena->offset];
    arena->offset += aligned_size;
    return ptr;
}
` + "\n")
}

func (cg *CodeGenerator) generateFunctionPrototype(node *GraphNode) {
	fnStmt, ok := node.SourceNode.(*FunctionStatement)
	if !ok {
		return
	}

	returnType := "void"

	var params []string
	params = append(params, "LeekArena* parent_arena")

	for _, p := range fnStmt.Parameters {
		params = append(params, fmt.Sprintf("%s %s", cg.mapType(p.Type), p.Name))
	}

	cg.builder.WriteString(fmt.Sprintf("%s %s(%s);\n", returnType, node.ID, strings.Join(params, ", ")))
}

func (cg *CodeGenerator) generateFunctionBody(node *GraphNode) error {
	fnStmt, ok := node.SourceNode.(*FunctionStatement)
	if !ok {
		return fmt.Errorf("node %s is not backed by a FunctionStatement", node.ID)
	}

	returnType := "void"
	cg.builder.WriteString(fmt.Sprintf("%s %s(LeekArena* parent_arena", returnType, node.ID))
	for _, p := range fnStmt.Parameters {
		cg.builder.WriteString(fmt.Sprintf(", %s %s", cg.mapType(p.Type), p.Name))
	}
	cg.builder.WriteString(") {\n")

	cg.builder.WriteString("    // Scope Entry: Allocate local arena bound to this lifecycle\n")
	cg.builder.WriteString("    uint8_t local_buffer[4096]; // Statisch vorhersehbare Größe pro Kausalpfad\n")
	cg.builder.WriteString("    LeekArena local_arena = { .buffer = local_buffer, .size = 4096, .offset = 0 };\n\n")

	cg.builder.WriteString("    // Internal Statement Flow\n")
	for _, stmt := range fnStmt.Body.Statements {
		cg.builder.WriteString(fmt.Sprintf("    // %s\n", stmt.String()))

		switch s := stmt.(type) {
		case *LetStatement:
			cType := cg.mapType(s.Type)

			cg.builder.WriteString(fmt.Sprintf("    %s* %s = (%s*)arena_alloc(&local_arena, sizeof(%s));\n",
				cType, s.Name, cType, cType))

			cg.builder.WriteString(fmt.Sprintf("    *%s = %s;\n\n", s.Name, s.Value))
		}
	}

	cg.builder.WriteString("\n    // Scope Exit: Local arena is atomized instantly, zero GC overhead\n")
	cg.builder.WriteString("    // local_arena collapses natively here via stack frame exit\n")
	cg.builder.WriteString("}\n\n")

	return nil
}

func (cg *CodeGenerator) mapType(leekType string) string {
	switch leekType {
	case "u8":
		return "uint8_t"
	case "u16":
		return "uint16_t"
	case "u32":
		return "uint32_t"
	case "u64":
		return "uint64_t"
	case "i8":
		return "int8_t"
	case "i16":
		return "int16_t"
	case "i32":
		return "int32_t"
	case "i64":
		return "int64_t"
	case "f32":
		return "float"
	case "f64":
		return "double"
	case "bool":
		return "int"
	default:
		return "void*" 
	}
}
