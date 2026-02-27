-- Pipe operator |>

let double n = n * 2

let inc n = n + 1

let square n = n * n


-- 3 |> inc |> double |> square  =>  (3+1)*2 = 8, 8^2 = 64
print (3 |> inc |> double |> square)
