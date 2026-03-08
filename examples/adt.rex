-- Algebraic data types

type Option = None | Some int
type List = Nil | Cons int List


length xs =
    match xs
        when Nil ->
            0
        when Cons _ t ->
            1 + length t


test "custom list length" =
    assert (Cons 1 (Cons 2 (Cons 3 Nil)) |> length == 3)
    assert (length Nil == 0)

test "option pattern match" =
    let get opt =
        match opt
            when None ->
                0
            when Some x ->
                x
    assert (Some 42 |> get == 42)
    assert (get None == 0)
