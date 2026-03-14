package rexfiles

var BitwiseFFI = map[string]any{
	"bitAnd":     Bitwise_bitAnd,
	"bitOr":      Bitwise_bitOr,
	"bitXor":     Bitwise_bitXor,
	"bitNot":     Bitwise_bitNot,
	"shiftLeft":  Bitwise_shiftLeft,
	"shiftRight": Bitwise_shiftRight,
}

func Bitwise_bitAnd(a, b int) int    { return a & b }
func Bitwise_bitOr(a, b int) int     { return a | b }
func Bitwise_bitXor(a, b int) int    { return a ^ b }
func Bitwise_bitNot(a int) int       { return ^a }
func Bitwise_shiftLeft(a, n int) int  { return a << uint(n) }
func Bitwise_shiftRight(a, n int) int { return a >> uint(n) }
