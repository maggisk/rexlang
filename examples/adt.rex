type Option = None | Some int
type List = Nil | Cons int List


let rec length xs =
    case xs of
        Nil ->
            0
        Cons _ t ->
            1 + length t


print (length (Cons 1 (Cons 2 (Cons 3 Nil))))
