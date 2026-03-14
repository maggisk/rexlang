package rexfiles

import "github.com/maggisk/rexlang/internal/eval"

func init() {
	eval.RegisterFFI("Bitwise", BitwiseFFI)
	eval.RegisterFFI("Math", MathFFI)
	eval.RegisterFFI("String", StringFFI)
	eval.RegisterFFI("IO", IOFFI)
	eval.RegisterFFI("Env", EnvFFI)
	eval.RegisterFFI("DateTime", DateTimeFFI)
	eval.RegisterFFI("Parallel", ParallelFFI)
	eval.RegisterFFI("Random", RandomFFI)
	eval.RegisterFFI("Process", ProcessFFI)
	eval.RegisterFFI("List", ListFFI)
	eval.RegisterFFI("Result", ResultFFI)
	eval.RegisterFFI("Json", JsonFFI)
	eval.RegisterFFI("Net", NetFFI)
	eval.RegisterFFI("Http.Server", HttpServerFFI)
}
