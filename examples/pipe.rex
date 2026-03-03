-- Pipe operator |>

let double n = n * 2

let inc n = n + 1

let square n = n * n


test "pipe operator" =
    -- 3 |> inc |> double |> square  =>  (3+1)*2 = 8, 8^2 = 64
    assert (3 |> inc |> double |> square == 64)
    assert (10 |> double == 20)
    assert (5 |> inc |> inc == 7)
