-- Actor-based chat room: demonstrates spawn, send, receive, call

import Std:Process (spawn, send, receive, call)
import Std:List (map, foldl, reverse)

-- Room actor manages a list of messages
type RoomMsg
    = Post String
    | GetHistory (Pid (List String))

roomActor me =
    let rec loop messages =
            match receive me
                when Post text ->
                    loop (text :: messages)
                when GetHistory replyPid ->
                    let _ = send replyPid (reverse messages)
                    in loop messages
    in loop []

test "chat room collects messages" =
    let room = spawn roomActor
    in let _ = send room (Post "hello")
    in let _ = send room (Post "world")
    in let history = call room (\rp -> GetHistory rp)
    in assert (history == ["hello", "world"])

test "empty room has no history" =
    let room = spawn roomActor
    in let history = call room (\rp -> GetHistory rp)
    in assert (history == [])
