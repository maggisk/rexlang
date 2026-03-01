import std:Map as M

let m = M.insert 3 30 (M.insert 2 20 (M.insert 1 10 M.empty))

M.foldl (fn k v acc -> acc + v) 0 m
