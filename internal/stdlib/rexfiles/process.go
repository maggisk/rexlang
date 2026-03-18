//go:build ignore

package main

// Process companions are thin wrappers around the concurrency runtime
// defined in the generated main.go (rex_spawn, rex_send, rex_call, rexGetSelf).

func Stdlib_Process_spawn(f any) any    { return rex_spawn(f) }
func Stdlib_Process_send(pid, msg any) any { return rex_send(pid, msg) }
func Stdlib_Process_receive(_ any) any  { return <-rexGetSelf().ch }
func Stdlib_Process_call(pid, fn any) any { return rex_call(pid, fn) }
