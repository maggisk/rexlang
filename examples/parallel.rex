-- Parallel map: apply a function to list elements concurrently.

import Std:Parallel (pmap, pmapN, numCPU)

square x = x * x

test "pmap" =
    assert (pmap square [1, 2, 3, 4, 5] == [1, 4, 9, 16, 25])
    assert (pmap (\x -> x + 1) [] == [])

test "pmapN" =
    assert (pmapN numCPU square [1, 2, 3, 4, 5, 6, 7, 8] == [1, 4, 9, 16, 25, 36, 49, 64])
    assert (pmapN 2 square [1, 2, 3] == [1, 4, 9])
