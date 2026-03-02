-- Algebraic data types

type Option = None | Some int
type List = Nil | Cons int List


let rec length xs =
    case xs of
        Nil ->
            0
        Cons _ t ->
            1 + length t


test "custom list length" =
    assert (Cons 1 (Cons 2 (Cons 3 Nil)) |> length == 3)
    assert (length Nil == 0)

test "option pattern match" =
    let get opt =
        case opt of
            None ->
                0
            Some x ->
                x
    assert (Some 42 |> get == 42)
    assert (get None == 0)

true
