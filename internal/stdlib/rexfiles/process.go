//go:build ignore

package main

// Process companions are thin wrappers around the concurrency runtime
// defined in the generated main.go (rex_spawn, rex_send, rex_call, rexGetSelf).

func Std_Process_spawn(f any) any       { return rex_spawn(f) }
func Std_Process_send(pid, msg any) any { return rex_send(pid, msg) }
func Std_Process_receive(_ any) any     { return <-rexGetSelf().ch }
func Std_Process_call(pid, fn any) any  { return rex_call(pid, fn) }

// monitor starts a goroutine that waits for the target process to exit,
// then sends the given message to the watcher's mailbox.
func Std_Process_monitor(watcherPid, targetPid, msg any) any {
	watcher := watcherPid.(*RexPid)
	target := targetPid.(*RexPid)
	go func() {
		<-target.done
		watcher.ch <- msg
	}()
	return nil
}
