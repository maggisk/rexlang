-- Higher-order functions


let apply f x = f x
let compose f g x = f (g x)

let double n = n * 2
let inc n = n + 1

print (compose double inc 20)
