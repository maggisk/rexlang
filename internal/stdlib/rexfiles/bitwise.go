//go:build ignore

package main

func Std_Bitwise_bitAnd(a, b int64) int64    { return a & b }
func Std_Bitwise_bitOr(a, b int64) int64     { return a | b }
func Std_Bitwise_bitXor(a, b int64) int64    { return a ^ b }
func Std_Bitwise_bitNot(a int64) int64       { return ^a }
func Std_Bitwise_shiftLeft(a, n int64) int64  { return a << uint(n) }
func Std_Bitwise_shiftRight(a, n int64) int64 { return a >> uint(n) }
