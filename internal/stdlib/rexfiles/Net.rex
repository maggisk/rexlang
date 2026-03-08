import Std:Result (Ok, Err)
import Std:IO (println)

export tcpListen, tcpAccept, tcpConnect, tcpRead, tcpWrite, tcpClose, tcpCloseListener


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
