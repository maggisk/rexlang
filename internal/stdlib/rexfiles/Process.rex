export external spawn : (() -> b) -> Pid a

export external send : Pid a -> a -> ()

export external receive : () -> a

export external self : Pid a

export external call : Pid b -> (Pid a -> b) -> a

test "spawn and call" =
    let pid = spawn \_ ->
            let caller = receive ()
            in send caller 42
    let n = call pid (\me -> me)
    assert (n == 42)

test "send and receive" =
    -- Capture self before spawning so the goroutine can reply to us.
    let me = self
    let pid = spawn \_ ->
            let msg = receive ()
            in send me msg
    let _ = send pid 99
    let reply = receive ()
    assert (reply == 99)
