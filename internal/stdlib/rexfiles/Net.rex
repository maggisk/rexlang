import Std:Result (Ok, Err)
import Std:IO (println)

export external tcpListen : Int -> Result (Listener, Int) String

export external tcpAccept : Listener -> Result Conn String

export external tcpConnect : String -> Int -> Result Conn String

export external tcpRead : Conn -> Result String String

export external tcpWrite : Conn -> String -> Result () String

export external tcpClose : Conn -> Result () String

export external tcpCloseListener : Listener -> Result () String


unwrap result =
    match result
        when Ok v ->
            v
        when Err e ->
            error ("unwrap failed: " ++ e)

test "echo round-trip on port 0" =
    match tcpListen 0
        when Err e ->
            let _ = println ("  [skip] tcpListen blocked: " ++ e)
            in ()
        when Ok (ln, port) ->
            let
                client = unwrap (tcpConnect "127.0.0.1" port)
                server = unwrap (tcpAccept ln)
                _ = unwrap (tcpWrite client "hello")
                msg = unwrap (tcpRead server)
                _ = unwrap (tcpClose client)
                _ = unwrap (tcpClose server)
                _ = unwrap (tcpCloseListener ln)
            in
            assert (msg == "hello")
