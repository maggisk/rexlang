export spawn, send, receive, self, call


test "spawn and call" =
    let pid = spawn (\_ ->
            let caller = receive () in
            send caller 42)
    let n = call pid (\me -> me)
    assert (n == 42)

test "send and receive" =
    -- Capture self before spawning so the goroutine can reply to us.
    let me = self
    let pid = spawn (\_ ->
            let msg = receive () in
            send me msg)
    let _ = send pid 99
    let reply = receive ()
    assert (reply == 99)
