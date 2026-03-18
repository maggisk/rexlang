package codegen

// runtimeSource is the static Go runtime that gets extracted to every build directory.
// It provides types and helpers that companion files and generated code both depend on.
const runtimeSource = `package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

type RexList struct {
	Head any
	Tail *RexList
}

// ---------------------------------------------------------------------------
// Tuples
// ---------------------------------------------------------------------------

type Tuple2 struct{ F0, F1 any }
type Tuple3 struct{ F0, F1, F2 any }
type Tuple4 struct{ F0, F1, F2, F3 any }

// ---------------------------------------------------------------------------
// Equality
// ---------------------------------------------------------------------------

func rex_eq(a, b any) bool {
	switch av := a.(type) {
	case int64:
		bv, ok := b.(int64); return ok && av == bv
	case float64:
		bv, ok := b.(float64); return ok && av == bv
	case string:
		bv, ok := b.(string); return ok && av == bv
	case bool:
		bv, ok := b.(bool); return ok && av == bv
	case nil:
		return b == nil
	case *RexList:
		bv, ok := b.(*RexList)
		if !ok { return false }
		if av == nil && bv == nil { return true }
		if av == nil || bv == nil { return false }
		return rex_eq(av.Head, bv.Head) && rex_eq(av.Tail, bv.Tail)
	default:
		return reflect.DeepEqual(a, b)
	}
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

func rex_compare(a, b any) int {
	switch av := a.(type) {
	case int64:
		bv := b.(int64)
		if av < bv { return -1 }
		if av > bv { return 1 }
		return 0
	case float64:
		bv := b.(float64)
		if av < bv { return -1 }
		if av > bv { return 1 }
		return 0
	case string:
		bv := b.(string)
		if av < bv { return -1 }
		if av > bv { return 1 }
		return 0
	case bool:
		bv := b.(bool)
		ai, bi := 0, 0
		if av { ai = 1 }
		if bv { bi = 1 }
		if ai < bi { return -1 }
		if ai > bi { return 1 }
		return 0
	}
	return 0
}

// ---------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------

func rex_display(v any) string {
	switch val := v.(type) {
	case nil:
		return "()"
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case string:
		return val
	case bool:
		if val { return "true" }
		return "false"
	case *RexList:
		if val == nil { return "[]" }
		var parts []string
		for l := val; l != nil; l = l.Tail {
			parts = append(parts, rex_display(l.Head))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ---------------------------------------------------------------------------
// Core builtins
// ---------------------------------------------------------------------------

func rex_showInt(v any) any {
	return strconv.FormatInt(v.(int64), 10)
}

func rex_showFloat(v any) any {
	return strconv.FormatFloat(v.(float64), 'g', -1, 64)
}

func rex_not(v any) any {
	return !v.(bool)
}

func rex_error(msg any) any {
	panic(fmt.Sprintf("error: %s", msg.(string)))
}

func rex_todo(msg any) any {
	panic(fmt.Sprintf("TODO: %s", msg.(string)))
}

// ---------------------------------------------------------------------------
// Function application
// ---------------------------------------------------------------------------

func rex__apply(f any, arg any) any {
	switch fn := f.(type) {
	case func(any) any:
		return fn(arg)
	default:
		panic("rex__apply: not a function")
	}
}

// ---------------------------------------------------------------------------
// Actor runtime
// ---------------------------------------------------------------------------

type RexPid struct {
	ch chan any
	id int64
}

var rexPidCounter int64
var rexPidMu sync.Mutex

var rexGoroutineSelf sync.Map

func rexNextPidID() int64 {
	rexPidMu.Lock()
	rexPidCounter++
	id := rexPidCounter
	rexPidMu.Unlock()
	return id
}

var rexMainPid = &RexPid{ch: make(chan any, 1024), id: 0}

func rexGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])
	s = s[len("goroutine "):]
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			s = s[:i]
			break
		}
	}
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

func init() {
	rexGoroutineSelf.Store(rexGoroutineID(), rexMainPid)
}

func rexGetSelf() *RexPid {
	v, ok := rexGoroutineSelf.Load(rexGoroutineID())
	if !ok { return rexMainPid }
	return v.(*RexPid)
}

func rex_spawn(f any) any {
	pid := &RexPid{ch: make(chan any, 1024), id: rexNextPidID()}
	go func() {
		rexGoroutineSelf.Store(rexGoroutineID(), pid)
		f.(func(any) any)(nil)
	}()
	return pid
}

func rex_send(pid any, msg any) any {
	pid.(*RexPid).ch <- msg
	return nil
}

func rex_call(targetPid any, msgFn any) any {
	replyPid := &RexPid{ch: make(chan any, 1), id: rexNextPidID()}
	fn := msgFn.(func(any) any)
	msg := fn(replyPid)
	targetPid.(*RexPid).ch <- msg
	return <-replyPid.ch
}
`
