import Std:Math (toFloat)
import Std:Bitwise (bitXor, bitAnd, shiftLeft, shiftRight)
import Std:Process (spawn, send, receive, call)
import Std:List (map, length, take, drop, concat, reverse)


-- # Pure RNG API

-- Opaque RNG state — consumers can't see it's just an Int
export opaque type Rng = | Rng Int


-- Mask to 32 bits (xorshift32 operates on unsigned 32-bit state)
mask32 : Int -> Int
mask32 n = bitAnd n 0xFFFFFFFF


-- | Create RNG from an integer seed. Seed must be non-zero after masking.
export
rngMake : Int -> Rng
rngMake seed =
    let s = mask32 seed
    in
    if s == 0 then
        Rng 1
    else
        Rng s

test "rngMake normalizes seed" =
    let rng = rngMake 0
    let (n, _) = rngInt 1 100 rng
    assert n >= 1
    assert n <= 100


-- | Create RNG from system entropy (impure).
export
rngFromSystem : () -> Rng
rngFromSystem _ = rngMake (systemSeed ())


-- Xorshift32 step
step : Rng -> (Int, Rng)
step rng =
    match rng
        when Rng state ->
            let
                s1 = mask32 (bitXor state (shiftLeft state 13))
                s2 = mask32 (bitXor s1 (shiftRight s1 17))
                s3 = mask32 (bitXor s2 (shiftLeft s2 5))
            in (s3, Rng s3)

test "deterministic sequence" =
    let rng = rngMake 42
    let (val1, rng2) = rngInt 1 100 rng
    let (val2, _) = rngInt 1 100 rng2
    let rng3 = rngMake 42
    let (val3, rng4) = rngInt 1 100 rng3
    let (val4, _) = rngInt 1 100 rng4
    assert val1 == val3
    assert val2 == val4


-- | Random int in [lo, hi] inclusive.
export
rngInt : Int -> Int -> Rng -> (Int, Rng)
rngInt lo hi rng =
    let
        (raw, rng2) = step rng
        range = hi - lo + 1
        result = (raw % range) + lo
    in (result, rng2)

test "int range" =
    let rng = rngMake 1
    let (n, _) = rngInt 1 6 rng
    assert n >= 1
    assert n <= 6


-- | Random float in [0.0, 1.0).
export
rngFloat : Rng -> (Float, Rng)
rngFloat rng =
    let (raw, rng2) = step rng
    in (toFloat raw / 4294967296.0, rng2)

test "float range" =
    let rng = rngMake 1
    let (f, _) = rngFloat rng
    assert f >= 0.0
    assert f < 1.0


-- | Random bool.
export
rngBool : Rng -> (Bool, Rng)
rngBool rng =
    let (n, rng2) = rngInt 0 1 rng
    in (n == 1, rng2)

test "rngBool returns bool" =
    let rng = rngMake 1
    let (val, _) = rngBool rng
    assert (val == true || val == false)


-- | Generate n random values using a generator function.
export
rngList : Int -> (Rng -> (a, Rng)) -> Rng -> ([a], Rng)
rngList n gen rng =
    let rec go remaining acc r =
        if remaining <= 0 then
            (reverse acc, r)
        else
            let (val, r2) = gen r
            in go (remaining - 1) (val :: acc) r2
    in go n [] rng

test "rngList generates correct count" =
    let rng = rngMake 42
    let (vals, _) = rngList 5 (\r -> rngInt 1 10 r) rng
    assert length vals == 5


-- # Actor Facade

type RngRequest
    = ReqInt Int Int (Pid Int)
    | ReqFloat (Pid Float)
    | ReqBool (Pid Bool)

rngActor = spawn \_ ->
    let rec loop rng =
        match receive ()
            when ReqInt lo hi replyPid ->
                let
                    (n, rng2) = rngInt lo hi rng
                    _ = send replyPid n
                in loop rng2
            when ReqFloat replyPid ->
                let
                    (f, rng2) = rngFloat rng
                    _ = send replyPid f
                in loop rng2
            when ReqBool replyPid ->
                let
                    (bval, rng2) = rngBool rng
                    _ = send replyPid bval
                in loop rng2
    in loop (rngFromSystem ())


-- | Random int in [lo, hi] inclusive (uses actor).
export
randomInt : Int -> Int -> Int
randomInt lo hi = call rngActor (\me -> ReqInt lo hi me)

test "randomInt works" =
    let n = randomInt 1 100
    assert n >= 1
    assert n <= 100


-- | Random float in [0.0, 1.0) (uses actor).
export
randomFloat : () -> Float
randomFloat _ = call rngActor (\me -> ReqFloat me)

test "randomFloat works" =
    let f = randomFloat ()
    assert f >= 0.0
    assert f < 1.0


-- | Random bool (uses actor).
export
randomBool : () -> Bool
randomBool _ = call rngActor (\me -> ReqBool me)

test "randomBool works" =
    let val = randomBool ()
    assert (val == true || val == false)


-- # Shuffle

-- | Shuffle a list using Fisher-Yates selection.
export
shuffle : [a] -> [a]
shuffle lst =
    let n = length lst
    in
    if n <= 1 then
        lst
    else
        let rec go acc remaining =
            match remaining
                when [] ->
                    acc
                when _ ->
                    let
                        idx = randomInt 0 (length remaining - 1)
                        picked = nth idx remaining
                        rest = removeAt idx remaining
                    in go (picked :: acc) rest
        in go [] lst

test "shuffle preserves length" =
    let original = [1, 2, 3, 4, 5]
    let shuffled = shuffle original
    assert length shuffled == length original

test "shuffle empty list" =
    let result = shuffle []
    assert result == []

test "shuffle single element" =
    let result = shuffle [42]
    assert result == [42]


-- # Helpers

nth : Int -> [a] -> a
nth i lst =
    match lst
        when [] ->
            error "nth: index out of bounds"
        when [h | t] ->
            if i == 0 then
                h
            else
                nth (i - 1) t

removeAt : Int -> [a] -> [a]
removeAt i lst =
    match lst
        when [] ->
            []
        when [h | t] ->
            if i == 0 then
                t
            else
                h :: removeAt (i - 1) t
