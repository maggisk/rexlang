package rexfiles

// Registry maps module name -> function name -> Go function (or constant value).
// Each companion .go file exports a map; this var collects them all.
var Registry = map[string]map[string]any{
	"Bitwise":  BitwiseFFI,
	"Math":     MathFFI,
	"String":   StringFFI,
	"IO":       IOFFI,
	"Env":      EnvFFI,
	"DateTime": DateTimeFFI,
	"Parallel": ParallelFFI,
	"Random":   RandomFFI,
}
