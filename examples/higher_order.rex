let apply f x = f x;;
let compose f g x = f (g x);;

let double n = n * 2;;
let inc n = n + 1;;

compose double inc 20
