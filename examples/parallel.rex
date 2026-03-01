-- Parallel map: apply a function to list elements concurrently.

import std:Parallel (pmap, pmapN, numCPU)
import std:IO (println)

let square x = x * x

-- One process per element
let a = pmap square [1, 2, 3, 4, 5]
let _ = println "pmap square [1..5]:"
let _ = println "  ${a}"

-- Bounded parallelism (numCPU workers)
let b = pmapN numCPU square [1, 2, 3, 4, 5, 6, 7, 8]
let _ = println "pmapN numCPU square [1..8]:"
let _ = println "  ${b}"

a == [1, 4, 9, 16, 25]
