import std:IO (println)

let name = "Rex"
let version = 1
let pi = 3.14

println "Hello, ${name}!"
println "Version ${version}, pi is ${pi}"
println "Escaped: \${not interpolated}"
println "Expr: ${1 + 2 + 3}"
println "Bool: ${true}"
println "Nested: ${"hello " ++ show 42}"

"done"
