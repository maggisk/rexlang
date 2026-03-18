//go:build ignore

package main

func Stdlib_Bitwise_bitAnd(a, b int64) int64    { return a & b }
func Stdlib_Bitwise_bitOr(a, b int64) int64     { return a | b }
func Stdlib_Bitwise_bitXor(a, b int64) int64    { return a ^ b }
func Stdlib_Bitwise_bitNot(a int64) int64       { return ^a }
func Stdlib_Bitwise_shiftLeft(a, n int64) int64  { return a << uint(n) }
func Stdlib_Bitwise_shiftRight(a, n int64) int64 { return a >> uint(n) }
