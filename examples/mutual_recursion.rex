-- Mutual recursion: isEven and isOdd

let rec isEven n =
    if n == 0 then
        true
    else
        isOdd (n - 1)
and isOdd n =
    if n == 0 then
        false
    else
        isEven (n - 1)

print (isEven 10)
