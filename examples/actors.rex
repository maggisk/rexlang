-- Actor model example: a stateful counter with typed messages.
--
-- The Msg ADT defines the protocol:
--   Inc          — fire-and-forget, bumps the counter
--   Get (Pid Int) — request-reply, sends current count to the caller
--   Stop         — graceful shutdown

import Std:IO (println)
import Std:String (toString)

type Msg = Inc | Get (Pid Int) | Stop

let counter =
    spawn \_ ->
        let rec loop n =
            case receive () of
                Inc ->
                    loop (n + 1)
                Get replyTo ->
                    let _ = send replyTo n in
                    loop n
                Stop ->
                    ()
        in
        loop 0

let _ = send counter Inc
let _ = send counter Inc
let _ = send counter Inc
let n = call counter Get
let _ = send counter Stop

export let main _ =
    let _ = n |> toString |> println in
    0
