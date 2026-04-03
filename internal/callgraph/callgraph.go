// Package callgraph builds a function dependency graph from a lowered IR program
// and emits it as a Graphviz DOT file.
package callgraph

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
)

// Node represents a function in the call graph.
type Node struct {
	Name       string // mangled name (e.g., "Std$List$map")
	Module     string // display module (e.g., "Std:List", "" for user)
	Short      string // short display name (e.g., "map")
	IsExternal bool   // Go-backed builtin
}

// Edge represents a call/reference from one function to another.
type Edge struct {
	From string
	To   string
}

// Graph is a function dependency graph.
type Graph struct {
	Nodes map[string]*Node
	Edges []Edge
}

// Build constructs a call graph from a lowered IR program.
// It collects all top-level function definitions as nodes and all
// variable references between them as edges.
func Build(prog *ir.Program) *Graph {
	g := &Graph{Nodes: make(map[string]*Node)}

	// First pass: register all top-level definitions as nodes.
	for _, d := range prog.Decls {
		switch dl := d.(type) {
		case ir.DLet:
			if dl.Name != "_" {
				g.addNode(dl.Name, false)
			}
		case ir.DLetRec:
			for _, b := range dl.Bindings {
				g.addNode(b.Name, false)
			}
		case ir.DExternal:
			g.addNode(dl.Name, true)
		}
	}

	// Second pass: collect edges from function bodies.
	seen := make(map[[2]string]bool)
	addEdge := func(from, to string) {
		if from == to {
			return // skip self-loops for cleaner graphs
		}
		key := [2]string{from, to}
		if seen[key] {
			return
		}
		if _, ok := g.Nodes[to]; !ok {
			return // target not a top-level definition
		}
		seen[key] = true
		g.Edges = append(g.Edges, Edge{From: from, To: to})
	}

	for _, d := range prog.Decls {
		switch dl := d.(type) {
		case ir.DLet:
			if dl.Name == "_" {
				continue
			}
			for _, ref := range collectRefs(dl.Body) {
				addEdge(dl.Name, ref)
			}
		case ir.DLetRec:
			for _, b := range dl.Bindings {
				for _, ref := range collectCExprRefs(b.Bind) {
					addEdge(b.Name, ref)
				}
			}
		}
	}

	return g
}

func (g *Graph) addNode(name string, isExternal bool) {
	if _, ok := g.Nodes[name]; ok {
		return
	}
	mod, short := splitName(name)
	g.Nodes[name] = &Node{
		Name:       name,
		Module:     mod,
		Short:      short,
		IsExternal: isExternal,
	}
}

// splitName extracts module and short name from a mangled IR name.
// "Std$List$map" → ("Std:List", "map")
// "myFunc" → ("", "myFunc")
func splitName(name string) (module, short string) {
	parts := strings.Split(name, "$")
	if len(parts) == 1 {
		return "", name
	}
	short = parts[len(parts)-1]
	module = strings.Join(parts[:len(parts)-1], ":")
	return module, short
}

// IsUserFunc returns true if the name is a user-defined function (not from stdlib/packages).
func IsUserFunc(name string) bool {
	return !strings.Contains(name, "$") && name != "_"
}

// WriteDot writes the call graph as a Graphviz DOT file.
func (g *Graph) WriteDot(w io.Writer) {
	fmt.Fprintln(w, "digraph callgraph {")
	fmt.Fprintln(w, `    rankdir=LR;`)
	fmt.Fprintln(w, `    node [fontname="Helvetica", fontsize=11];`)
	fmt.Fprintln(w, `    edge [color="#666666"];`)
	fmt.Fprintln(w)

	// Group nodes by module.
	modules := make(map[string][]string)
	for name, node := range g.Nodes {
		modules[node.Module] = append(modules[node.Module], name)
	}

	// Sort module names for deterministic output.
	var modNames []string
	for m := range modules {
		modNames = append(modNames, m)
	}
	sort.Strings(modNames)

	for _, mod := range modNames {
		names := modules[mod]
		sort.Strings(names)

		if mod == "" {
			// User-defined functions — no cluster, prominent styling.
			for _, name := range names {
				node := g.Nodes[name]
				style := `shape=box, style="filled,rounded", fillcolor="#d4edda"`
				if name == "main" {
					style = `shape=box, style="filled,bold,rounded", fillcolor="#b8daff", penwidth=2`
				}
				fmt.Fprintf(w, "    %s [label=%q, %s];\n", dotID(name), node.Short, style)
			}
		} else {
			// Stdlib/package module — cluster with subdued styling.
			clusterID := strings.NewReplacer(":", "_", "$", "_", ".", "_").Replace(mod)
			fmt.Fprintf(w, "    subgraph cluster_%s {\n", clusterID)
			fmt.Fprintf(w, "        label=%q;\n", mod)
			fmt.Fprintf(w, "        style=filled;\n")
			fmt.Fprintf(w, `        fillcolor="#f8f9fa";`)
			fmt.Fprintln(w)
			fmt.Fprintf(w, `        color="#dee2e6";`)
			fmt.Fprintln(w)
			for _, name := range names {
				node := g.Nodes[name]
				style := `shape=ellipse, style=filled, fillcolor="#e9ecef"`
				if node.IsExternal {
					style = `shape=ellipse, style="filled,dashed", fillcolor="#e9ecef"`
				}
				fmt.Fprintf(w, "        %s [label=%q, %s];\n", dotID(name), node.Short, style)
			}
			fmt.Fprintln(w, "    }")
		}
		fmt.Fprintln(w)
	}

	// Edges.
	for _, e := range g.Edges {
		fmt.Fprintf(w, "    %s -> %s;\n", dotID(e.From), dotID(e.To))
	}

	fmt.Fprintln(w, "}")
}

// dotID wraps a name as a quoted DOT identifier.
func dotID(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
}

// ---------------------------------------------------------------------------
// IR walkers — collect all variable references in function bodies
// ---------------------------------------------------------------------------

func collectRefs(expr ir.Expr) []string {
	var refs []string
	walkExpr(expr, &refs)
	return refs
}

func collectCExprRefs(c ir.CExpr) []string {
	var refs []string
	walkCExpr(c, &refs)
	return refs
}

func walkExpr(expr ir.Expr, refs *[]string) {
	switch e := expr.(type) {
	case ir.EAtom:
		walkAtom(e.A, refs)
	case ir.EComplex:
		walkCExpr(e.C, refs)
	case ir.ELet:
		walkCExpr(e.Bind, refs)
		walkExpr(e.Body, refs)
	case ir.ELetRec:
		for _, b := range e.Bindings {
			walkCExpr(b.Bind, refs)
		}
		walkExpr(e.Body, refs)
	}
}

func walkCExpr(c ir.CExpr, refs *[]string) {
	switch e := c.(type) {
	case ir.CApp:
		walkAtom(e.Func, refs)
		walkAtom(e.Arg, refs)
	case ir.CBinop:
		walkAtom(e.Left, refs)
		walkAtom(e.Right, refs)
	case ir.CUnaryMinus:
		walkAtom(e.Expr, refs)
	case ir.CIf:
		walkAtom(e.Cond, refs)
		walkExpr(e.Then, refs)
		walkExpr(e.Else, refs)
	case ir.CMatch:
		walkAtom(e.Scrutinee, refs)
		for _, arm := range e.Arms {
			walkExpr(arm.Body, refs)
		}
	case ir.CLambda:
		walkExpr(e.Body, refs)
	case ir.CCtor:
		for _, a := range e.Args {
			walkAtom(a, refs)
		}
	case ir.CRecord:
		for _, f := range e.Fields {
			walkAtom(f.Value, refs)
		}
	case ir.CFieldAccess:
		walkAtom(e.Record, refs)
	case ir.CRecordUpdate:
		walkAtom(e.Record, refs)
		for _, u := range e.Updates {
			walkAtom(u.Value, refs)
		}
	case ir.CList:
		for _, a := range e.Items {
			walkAtom(a, refs)
		}
	case ir.CTuple:
		for _, a := range e.Items {
			walkAtom(a, refs)
		}
	case ir.CStringInterp:
		for _, a := range e.Parts {
			walkAtom(a, refs)
		}
	case ir.CAssert:
		walkAtom(e.Expr, refs)
	}
}

func walkAtom(a ir.Atom, refs *[]string) {
	if v, ok := a.(ir.AVar); ok {
		*refs = append(*refs, v.Name)
	}
}
