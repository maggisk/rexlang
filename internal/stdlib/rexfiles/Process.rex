export external spawn : (Pid a -> b) -> Pid a

export external send : Pid a -> a -> ()

export external receive : Pid a -> a

export external call : Pid b -> (Pid a -> b) -> a

-- | ProcessDown is sent to a monitoring process when the monitored process exits.
export type ProcessDown = ProcessDown

-- | monitor watcher target msg -- when target exits (normally or via error),
-- msg is sent to the watcher's mailbox.
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


-- # Monitor tests

type WatchMsg = Stopped | GetCount (Pid Int)

test "monitor sends message when process exits" =
    let
        watcher = spawn \me ->
            let rec loop count =
                match receive me
                    when Stopped ->
                        loop (count + 1)
                    when GetCount replyPid ->
                        let _ = send replyPid count
                        in loop count
            in loop 0
        worker = spawn (\_ -> ())
        _ = monitor watcher worker Stopped
    in let rec waitFor _ =
            let count = call watcher (\replyPid -> GetCount replyPid)
            in if count > 0 then
                count
            else
                waitFor ()
    in assert (waitFor () == 1)

test "monitor sends message when process panics" =
    let
        watcher = spawn \me ->
            let rec loop count =
                match receive me
                    when Stopped ->
                        loop (count + 1)
                    when GetCount replyPid ->
                        let _ = send replyPid count
                        in loop count
            in loop 0
        worker = spawn (\_ -> error "crash")
        _ = monitor watcher worker Stopped
    in let rec waitFor _ =
            let count = call watcher (\replyPid -> GetCount replyPid)
            in if count > 0 then
                count
            else
                waitFor ()
    in assert (waitFor () == 1)

test "multiple monitors on same process" =
    let
        watcher1 = spawn \me ->
            let rec loop count =
                match receive me
                    when Stopped ->
                        loop (count + 1)
                    when GetCount replyPid ->
                        let _ = send replyPid count
                        in loop count
            in loop 0
        watcher2 = spawn \me ->
            let rec loop count =
                match receive me
                    when Stopped ->
                        loop (count + 1)
                    when GetCount replyPid ->
                        let _ = send replyPid count
                        in loop count
            in loop 0
        worker = spawn (\_ -> ())
        _ = monitor watcher1 worker Stopped
        _ = monitor watcher2 worker Stopped
    in let rec waitFor1 _ =
            let count = call watcher1 (\replyPid -> GetCount replyPid)
            in if count > 0 then
                count
            else
                waitFor1 ()
    in let rec waitFor2 _ =
            let count = call watcher2 (\replyPid -> GetCount replyPid)
            in if count > 0 then
                count
            else
                waitFor2 ()
    in let _ = assert (waitFor1 () == 1)
    in assert (waitFor2 () == 1)
