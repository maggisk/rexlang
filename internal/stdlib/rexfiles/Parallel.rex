export pmap, pmapN, numCPU

import std:List (map, length, take, drop, concat)


-- | Apply a function to each element in parallel (one process per element).
-- Results are returned in the same order as the input list.
pmap : (a -> b) -> [a] -> [b]
let pmap f lst =
    let pids = map (fn x ->
        spawn (fn _ ->
            let result = f x
                caller = receive ()
            in
            send caller result
        )
    ) lst
    in
    map (fn pid -> call pid (fn me -> me)) pids


-- | Apply a function to each element in parallel, using at most n workers.
-- The list is split into n chunks; each chunk is processed by one worker.
pmapN : Int -> (a -> b) -> [a] -> [b]
let pmapN n f lst =
    let total = length lst
        size = if total == 0 then
                    1
                else
                    (total + n - 1) / n
    in
    let rec chunks l =
        case l of
            [] ->
                []
            _ ->
                take size l :: chunks (drop size l)
    in
    let pids = map (fn chunk ->
        spawn (fn _ ->
            let result = map f chunk
                caller = receive ()
            in
            send caller result
        )
    ) (chunks lst)
    in
    concat (map (fn pid -> call pid (fn me -> me)) pids)


test "pmap preserves order" =
    let result = pmap (fn x -> x * 2) [1, 2, 3, 4, 5]
    assert (result == [2, 4, 6, 8, 10])

test "pmap on empty list" =
    let result = pmap (fn x -> x + 1) []
    assert (result == [])

test "pmapN preserves order" =
    let result = pmapN 2 (fn x -> x * 10) [1, 2, 3, 4, 5]
    assert (result == [10, 20, 30, 40, 50])

test "pmapN on empty list" =
    let result = pmapN 4 (fn x -> x + 1) []
    assert (result == [])

test "pmapN with 1 worker" =
    let result = pmapN 1 (fn x -> x * x) [1, 2, 3]
    assert (result == [1, 4, 9])

test "numCPU is positive" =
    assert (numCPU > 0)
