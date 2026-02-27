-- Tuple type and let destructuring

let swap pair =
    let (a, b) = pair in
    (b, a)

let fst pair =
    let (a, _) = pair in
    a

let snd pair =
    let (_, b) = pair in
    b

let p = (10, 20)

let (x, y) = p

print (x + y)
