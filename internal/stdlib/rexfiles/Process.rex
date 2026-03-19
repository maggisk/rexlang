import Std:Maybe (Just, Nothing)

export external spawn : (Pid a -> b) -> Pid a

export external send : Pid a -> a -> ()

export external receive : Pid a -> a

export external receiveTimeout : Pid a -> Int -> Maybe a

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

test "receiveTimeout returns Nothing on timeout" =
    let pid = spawn \me ->
            let
                (replyPid, _) = receive me
                result = receiveTimeout me 10
            in match result
                when Nothing ->
                    send replyPid 0
                when Just _ ->
                    send replyPid 1
    in let answer = call pid (\replyPid -> (replyPid, ()))
    in assert (answer == 0)

test "receiveTimeout returns Just on message" =
    let
        dummy = spawn (\_ -> ())
        pid = spawn \me ->
                let
                    _ = receive me
                    result = receiveTimeout me 1000
                in match result
                    when Just (replyPid, _) ->
                        send replyPid 42
                    when Nothing ->
                        ()
    in let _ = send pid (dummy, 0)
    in let answer = call pid (\replyPid -> (replyPid, 0))
    in assert (answer == 42)
