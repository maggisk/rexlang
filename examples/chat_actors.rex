-- chat_actors.rex — Actor-based chat room
--
-- Demonstrates Rex's actor model with typed message passing.
-- A central "room" actor maintains a list of connected clients.
-- Client actors collect messages they receive. At the end, we
-- query each client for their message log — proving the broadcast
-- protocol works correctly.
--
-- Message types are ADTs — the type system ensures actors only
-- receive messages they can handle.

import Std:IO (println)
import Std:String (toString, join)
import Std:List (filter, indexedMap, foldl)
import Std:Process (spawn, send, receive, call)


-- # Message types


-- | Messages the room actor understands.
type RoomMsg
    = Join String (Pid ClientMsg)
    | Leave String
    | Broadcast String String
    | GetMembers (Pid [String])

-- | Messages a client actor receives.
type ClientMsg
    = Notify String
    | GetLog (Pid [String])


-- # Helpers

-- | Reverse a list via fold (avoids name collision with Std:String.reverse).
rev : [a] -> [a]
rev lst =
    foldl (\acc x -> x :: acc) [] lst

-- | Count elements in a list.
count : [a] -> Int
count lst =
    foldl (\acc _ -> acc + 1) 0 lst


-- # Room actor


-- | Each connected member has a name and a pid.
type Member = { name : String, pid : Pid ClientMsg }

-- | The room maintains a list of members and handles
-- join, leave, broadcast, and membership queries.
room =
    spawn \me ->
        let rec loop members =
            match receive me
                when Join name clientPid ->
                    let _ = members
                            |> indexedMap (\_ m ->
                                send m.pid (Notify "${name} joined"))
                    in
                    let newMember = Member { name = name, pid = clientPid }
                    in loop (newMember :: members)
                when Leave name ->
                    let remaining = members
                            |> filter (\m -> m.name != name)
                    in
                    let _ = remaining
                            |> indexedMap (\_ m ->
                                send m.pid (Notify "${name} left"))
                    in loop remaining
                when Broadcast sender text ->
                    let _ = members
                            |> filter (\m -> m.name != sender)
                            |> indexedMap (\_ m ->
                                send m.pid (Notify "${sender}: ${text}"))
                    in loop members
                when GetMembers replyTo ->
                    let names = members
                            |> indexedMap (\_ m -> m.name)
                    in
                    let _ = send replyTo names
                    in loop members
        in
        loop []


-- # Client actor


-- | A client collects notifications into a log.
-- Responds to GetLog with its accumulated messages.
makeClient : String -> Pid ClientMsg
makeClient name =
    spawn \me ->
        let rec loop log =
            match receive me
                when Notify msg ->
                    loop (msg :: log)
                when GetLog replyTo ->
                    let _ = send replyTo (rev log)
                    in loop log
        in
        loop []


-- | Query a client for its message log.
getClientLog : Pid ClientMsg -> [String]
getClientLog clientPid =
    call clientPid (\replyTo -> GetLog replyTo)


-- # Simulation


export
main : [String] -> Int
main _ =
    let
        -- Create clients
        alice = makeClient "Alice"
        bob = makeClient "Bob"
        charlie = makeClient "Charlie"

        -- Everyone joins the room
        _ = send room (Join "Alice" alice)
        _ = send room (Join "Bob" bob)
        _ = send room (Join "Charlie" charlie)

        -- Send some messages
        _ = send room (Broadcast "Alice" "Hello everyone!")
        _ = send room (Broadcast "Bob" "Hey Alice!")

        -- Charlie leaves
        _ = send room (Leave "Charlie")

        -- Message after Charlie left (only Alice and Bob receive it)
        _ = send room (Broadcast "Alice" "Charlie is gone")

        -- Query the room for current members
        members = call room (\replyTo -> GetMembers replyTo)

        -- Query each client for their message log
        aliceLog = getClientLog alice
        bobLog = getClientLog bob
        charlieLog = getClientLog charlie

        -- Print results
        _ = println "--- Room members ---"
        _ = println (members |> join ", ")
        _ = println ""
        _ = println "--- Alice's log ---"
        _ = aliceLog |> indexedMap (\_ msg -> println ("  " ++ msg))
        _ = println ""
        _ = println "--- Bob's log ---"
        _ = bobLog |> indexedMap (\_ msg -> println ("  " ++ msg))
        _ = println ""
        _ = println "--- Charlie's log ---"
        _ = charlieLog |> indexedMap (\_ msg -> println ("  " ++ msg))
        _ = println ""
        _ = println "Total messages: Alice=${toString (count aliceLog)}, Bob=${toString (count bobLog)}, Charlie=${toString (count charlieLog)}"
    in 0
