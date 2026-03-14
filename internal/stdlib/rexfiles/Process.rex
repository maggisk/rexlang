export external spawn : (Pid a -> b) -> Pid a

export external send : Pid a -> a -> ()

export external receive : Pid a -> a

export external call : Pid b -> (Pid a -> b) -> a

test "spawn and call" =
    let pid = spawn \me ->
            let (replyPid, value) = receive me
            in send replyPid (value * 2)
    in let result = call pid (\replyPid -> (replyPid, 21))
    in assert (result == 42)

test "send and receive" =
    let pid = spawn \me ->
            let rec loop total =
                    let (replyPid, n) = receive me
                    in if n == 0 then
                        send replyPid total
                    else
                        loop (total + n)
            in loop 0
    in let _ = send pid (spawn (\_ -> ()), 10)
    in let _ = send pid (spawn (\_ -> ()), 20)
    in let result = call pid (\replyPid -> (replyPid, 0))
    in assert (result == 30)
