package rexfiles

import "github.com/maggisk/rexlang/internal/eval"

var ProcessFFI = map[string]any{
	"receive": eval.MakeBuiltin("receive", func(pidV eval.Value) (eval.Value, error) {
		pid, ok := pidV.(eval.VPid)
		if !ok {
			return nil, eval.RuntimeErr("receive: expected Pid, got %s", eval.ValueToString(pidV))
		}
		return pid.Mailbox.Receive(), nil
	}),
	"send": eval.Curried2("send", func(pidV, msgV eval.Value) (eval.Value, error) {
		pid, ok := pidV.(eval.VPid)
		if !ok {
			return nil, eval.RuntimeErr("send: expected Pid, got %s", eval.ValueToString(pidV))
		}
		pid.Mailbox.Send(msgV)
		return eval.VUnit{}, nil
	}),
	"spawn": eval.MakeBuiltin("spawn", func(fnV eval.Value) (eval.Value, error) {
		mb := eval.NewMailbox()
		pid := eval.VPid{Mailbox: mb, ID: mb.ID}
		go func() {
			eval.ApplyValue(fnV, pid) //nolint:errcheck
		}()
		return pid, nil
	}),
	"call": eval.Curried2("call", func(pidV, makeMsgV eval.Value) (eval.Value, error) {
		pid, ok := pidV.(eval.VPid)
		if !ok {
			return nil, eval.RuntimeErr("call: expected Pid, got %s", eval.ValueToString(pidV))
		}
		replyMb := eval.NewMailbox()
		replyPid := eval.VPid{Mailbox: replyMb, ID: replyMb.ID}
		msg, err := eval.ApplyValue(makeMsgV, replyPid)
		if err != nil {
			return nil, err
		}
		pid.Mailbox.Send(msg)
		return replyMb.Receive(), nil
	}),
}
