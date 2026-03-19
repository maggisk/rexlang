//go:build ignore

package main

import "time"

// Process companions are thin wrappers around the concurrency runtime
// defined in the generated main.go (rex_spawn, rex_send, rex_call, rexGetSelf).

func Std_Process_spawn(f any) any      { return rex_spawn(f) }
func Std_Process_send(pid, msg any) any { return rex_send(pid, msg) }
func Std_Process_receive(_ any) any    { return <-rexGetSelf().ch }
func Std_Process_call(pid, fn any) any { return rex_call(pid, fn) }

// receiveTimeout waits up to ms milliseconds for a message.
// Returns *any (non-nil = Just, nil = Nothing) — the codegen's Maybe wrapper handles conversion.
func Std_Process_receiveTimeout(_ any, ms any) *any {
	timeout := time.Duration(ms.(int64)) * time.Millisecond
	select {
	case msg := <-rexGetSelf().ch:
		return &msg
	case <-time.After(timeout):
		return nil
	}
}
