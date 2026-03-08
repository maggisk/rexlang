-- Actor model example: a stateful counter with typed messages.
--
-- The Msg ADT defines the protocol:
--   Inc          — fire-and-forget, bumps the counter
--   Get (Pid Int) — request-reply, sends current count to the caller
--   Stop         — graceful shutdown

import Std:IO (println)
import Std:String (toString)
import Std:Process (spawn, send, receive, self, call)

type Msg = Inc | Get (Pid Int) | Stop

counter =
    spawn \_ ->
        let rec loop n =
            match receive ()
                when Inc ->
                    loop (n + 1)
                when Get replyTo ->
                    let _ = send replyTo n
                    in loop n
                when Stop ->
                    ()
        in
        loop 0

_ = send counter Inc
_ = send counter Inc
_ = send counter Inc
n = call counter Get
_ = send counter Stop

main _ =
    let _ = n |> toString |> println
    in 0
