-- TCP echo server example using actors.
-- Each client connection gets its own handler goroutine.
-- Run: ./rex examples/tcp_echo.rex
-- Test: echo "hello" | nc localhost 9000

import Std:IO (println)
import Std:String (toString)
import Std:Net (tcpListen, tcpAccept, tcpRead, tcpWrite, tcpClose, tcpCloseListener)
import Std:Process (spawn)
import Std:Result (Ok, Err)


-- Echo handler: reads from conn until EOF, echoes back each message
handleClient conn =
    case tcpRead conn of
        Err _ ->
            tcpClose conn
        Ok msg ->
            let _ = tcpWrite conn msg in
            handleClient conn


-- Accept loop: accepts connections and spawns a handler for each
acceptLoop ln =
    case tcpAccept ln of
        Err _ ->
            ()
        Ok conn ->
            let _ = spawn (\_ -> handleClient conn) in
            acceptLoop ln


export main _ =
    case tcpListen 9000 of
        Err e ->
            let _ = println "Failed to listen: ${e}" in
            1
        Ok (ln, port) ->
            let _ = println "Listening on port ${toString port}" in
            let _ = acceptLoop ln in
            let _ = tcpCloseListener ln in
            0
