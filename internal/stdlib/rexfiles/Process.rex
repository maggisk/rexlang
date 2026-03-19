import Std:Maybe (Just, Nothing)

export external spawn : (Pid a -> b) -> Pid a

export external send : Pid a -> a -> ()

export external receive : Pid a -> a

export external receiveTimeout : Pid a -> Int -> Maybe a

export external call : Pid b -> (Pid a -> b) -> a

export external monitor : Pid a -> Pid b -> a -> ()

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

test "monitor type checks" =
    let _ = spawn \me ->
            let worker = spawn \_ -> ()
            in let _ = monitor me worker 42
            in let msg = receive me
            in msg
    in assert (1 == 1)

test "receiveTimeout type checks" =
    let _ = spawn \me ->
            let r = receiveTimeout me 100
            in match r
                when Just _ ->
                    ()
                when Nothing ->
                    ()
    in assert (1 == 1)
