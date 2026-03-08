import Std:Result (Ok, Err)

export tcpListen, tcpAccept, tcpConnect, tcpRead, tcpWrite, tcpClose, tcpCloseListener


unwrap result =
    case result of
        Ok v ->
            v
        Err _ ->
            error "unwrap failed"

test "echo round-trip on port 0" =
    let (ln, port) = unwrap (tcpListen 0)
    let client = unwrap (tcpConnect "127.0.0.1" port)
    let server = unwrap (tcpAccept ln)
    let _ = unwrap (tcpWrite client "hello")
    let msg = unwrap (tcpRead server)
    assert (msg == "hello")
    let _ = unwrap (tcpClose client)
    let _ = unwrap (tcpClose server)
    let _ = unwrap (tcpCloseListener ln)
    ()
