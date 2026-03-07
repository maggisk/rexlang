-- Pipe operator |>

double n = n * 2

inc n = n + 1

square n = n * n


test "pipe operator" =
    -- 3 |> inc |> double |> square  =>  (3+1)*2 = 8, 8^2 = 64
    assert (3 |> inc |> double |> square == 64)
    assert (10 |> double == 20)
    assert (5 |> inc |> inc == 7)
