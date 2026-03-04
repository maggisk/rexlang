-- Main module: demonstrates user module imports

import Std:IO (println)
import Utils (double, greet)
import Lib.Helpers as H

export let main _ =
    let _ = println (greet "World") in
    let _ = println "double 21 = ${double 21}" in
    let _ = println "sumDoubles [1,2,3] = ${H.sumDoubles [1, 2, 3]}" in
    let _ = println "squares [1,2,3] = ${H.squares [1, 2, 3]}" in
    0
