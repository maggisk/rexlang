export empty, singleton, fromList, lookup, member, size, isEmpty, insert, remove, update, map, filter, foldl, foldr, toList, keys, values, mapWithKey, union, unionWith, intersect, intersectWith, difference, findMin, findMax, any, all


type Map k v = Empty | Node int Map k v Map


-- # Internal AVL helpers


-- | Get the height of a map node (Empty = 0).
let height m =
    case m of
        Empty ->
            0
        Node h _ _ _ _ ->
            h


-- | Smart constructor — computes height from children.
let node l k v r =
    let lh = height l
        rh = height r
        h = if lh > rh then
                lh + 1
            else
                rh + 1
    in
    Node h l k v r


-- | Balance factor (left height minus right height).
let balanceFactor m =
    case m of
        Empty ->
            0
        Node _ l _ _ r ->
            height l - height r


-- | Rotate right around the root.
let rotateRight m =
    case m of
        Node _ (Node _ a xk xv b) yk yv c ->
            node a xk xv (node b yk yv c)
        _ ->
            m


-- | Rotate left around the root.
let rotateLeft m =
    case m of
        Node _ a xk xv (Node _ b yk yv c) ->
            node (node a xk xv b) yk yv c
        _ ->
            m


-- | Rebalance a node after insertion or removal.
let rebalance m =
    case m of
        Empty ->
            Empty
        Node _ l k v r ->
            let bf = height l - height r in
            if bf > 1 then
                if balanceFactor l < 0 then
                    rotateRight (node (rotateLeft l) k v r)
                else
                    rotateRight m
            else if bf < -1 then
                if balanceFactor r > 0 then
                    rotateLeft (node l k v (rotateRight r))
                else
                    rotateLeft m
            else
                m


-- # Internal helpers (not exported)


let unwrap m =
    case m of
        Just v ->
            v
        Nothing ->
            error "unwrap: Nothing"


-- # Query


-- | Count the number of entries in the map.
size : Map k v -> Int
let rec size m =
    case m of
        Empty ->
            0
        Node _ l _ _ r ->
            1 + size l + size r


-- | Check if the map is empty.
isEmpty : Map k v -> Bool
let isEmpty m =
    case m of
        Empty ->
            true
        Node _ _ _ _ _ ->
            false


-- | Look up a key, returning Just value or Nothing.
lookup : k -> Map k v -> Maybe v
let rec lookup key m =
    case m of
        Empty ->
            Nothing
        Node _ l k v r ->
            case compare key k of
                LT ->
                    lookup key l
                GT ->
                    lookup key r
                EQ ->
                    Just v


-- | Check if a key is present in the map.
member : k -> Map k v -> Bool
let rec member key m =
    case m of
        Empty ->
            false
        Node _ l k _ r ->
            case compare key k of
                LT ->
                    member key l
                GT ->
                    member key r
                EQ ->
                    true


-- # Create


-- | An empty map.
let empty = Empty


-- | Create a map with a single key-value pair.
singleton : k -> v -> Map k v
let singleton k v = Node 1 Empty k v Empty

test "empty and singleton" =
    assert (isEmpty empty)
    assert (not (isEmpty (singleton 1 10)))
    assert (size (singleton 1 10) == 1)


-- # Modify


-- | Insert a key-value pair, replacing any existing value for the key.
insert : k -> v -> Map k v -> Map k v
let rec insert key val m =
    case m of
        Empty ->
            Node 1 Empty key val Empty
        Node h l k v r ->
            case compare key k of
                LT ->
                    rebalance (node (insert key val l) k v r)
                GT ->
                    rebalance (node l k v (insert key val r))
                EQ ->
                    Node h l key val r

test "insert" =
    let m = insert 2 20 (insert 1 10 empty)
    assert (size m == 2)
    assert (unwrap (lookup 1 m) == 10)
    assert (unwrap (lookup 2 m) == 20)
    assert (member 1 m)
    assert (not (member 3 m))

test "insert replaces existing key" =
    let m = insert 1 99 (insert 1 10 empty)
    assert (size m == 1)
    assert (unwrap (lookup 1 m) == 99)


-- | Remove the minimum element, returning (minKey, minValue, remaining).
let rec removeMin m =
    case m of
        Node _ (Empty) k v r ->
            (k, v, r)
        Node _ l k v r ->
            let (mk, mv, newL) = removeMin l in
            (mk, mv, rebalance (node newL k v r))
        Empty ->
            error "removeMin: empty map"


-- | Remove a key from the map.
remove : k -> Map k v -> Map k v
let rec remove key m =
    case m of
        Empty ->
            Empty
        Node _ l k v r ->
            case compare key k of
                LT ->
                    rebalance (node (remove key l) k v r)
                GT ->
                    rebalance (node l k v (remove key r))
                EQ ->
                    case r of
                        Empty ->
                            l
                        _ ->
                            let (mk, mv, newR) = removeMin r in
                            rebalance (node l mk mv newR)

test "remove" =
    let m = insert 3 30 (insert 2 20 (insert 1 10 empty))
    let m2 = remove 2 m
    assert (size m2 == 2)
    assert (member 1 m2)
    assert (not (member 2 m2))
    assert (member 3 m2)


-- | Update the value at a key by applying a function. No-op if key absent.
update : k -> (v -> v) -> Map k v -> Map k v
let rec update key f m =
    case m of
        Empty ->
            Empty
        Node h l k v r ->
            case compare key k of
                LT ->
                    Node h (update key f l) k v r
                GT ->
                    Node h l k v (update key f r)
                EQ ->
                    Node h l k (f v) r

test "update" =
    let m = insert 1 10 empty
    let m2 = update 1 (fn v -> v + 5) m
    assert (unwrap (lookup 1 m2) == 15)


-- | Build a map from a list of (key, value) pairs.
fromList : [(k, v)] -> Map k v
let fromList lst =
    let rec go acc pairs =
        case pairs of
            [] ->
                acc
            [pair | rest] ->
                let (k, v) = pair in
                go (insert k v acc) rest
    in
    go empty lst

test "fromList" =
    let m = fromList [(1, 10), (2, 20), (3, 30)]
    assert (size m == 3)
    assert (unwrap (lookup 2 m) == 20)


-- # Fold


-- | Fold over key-value pairs from smallest to largest key.
foldl : (k -> v -> a -> a) -> a -> Map k v -> a
let rec foldl f acc m =
    case m of
        Empty ->
            acc
        Node _ l k v r ->
            let acc1 = foldl f acc l
                acc2 = f k v acc1
            in
            foldl f acc2 r

test "foldl" =
    let m = fromList [(1, 10), (2, 20), (3, 30)]
    assert (foldl (fn k v acc -> acc + v) 0 m == 60)


-- | Fold over key-value pairs from largest to smallest key.
foldr : (k -> v -> a -> a) -> a -> Map k v -> a
let rec foldr f acc m =
    case m of
        Empty ->
            acc
        Node _ l k v r ->
            let acc1 = foldr f acc r
                acc2 = f k v acc1
            in
            foldr f acc2 l


-- # Convert


-- | Convert to a sorted list of (key, value) pairs.
toList : Map k v -> [(k, v)]
let toList m =
    foldr (fn k v acc -> (k, v) :: acc) [] m


-- | Get all keys in sorted order.
keys : Map k v -> [k]
let keys m =
    foldr (fn k v acc -> k :: acc) [] m


-- | Get all values in key order.
values : Map k v -> [v]
let values m =
    foldr (fn k v acc -> v :: acc) [] m

test "keys and values" =
    import std:List (length)
    let m = fromList [(3, 30), (1, 10), (2, 20)]
    assert (length (keys m) == 3)
    assert (length (values m) == 3)


-- # Transform


-- | Apply a function to every value in the map.
map : (v -> w) -> Map k v -> Map k w
let rec map f m =
    case m of
        Empty ->
            Empty
        Node h l k v r ->
            Node h (map f l) k (f v) (map f r)

test "map" =
    let m = fromList [(1, 10), (2, 20)]
    let m2 = map (fn v -> v * 2) m
    assert (unwrap (lookup 1 m2) == 20)
    assert (unwrap (lookup 2 m2) == 40)


-- | Apply a function to every key-value pair in the map.
--
--     mapWithKey (fn k v -> k + v) (fromList [(1, 10), (2, 20)]) == fromList [(1, 11), (2, 22)]
--
mapWithKey : (k -> v -> w) -> Map k v -> Map k w
let rec mapWithKey f m =
    case m of
        Empty ->
            Empty
        Node h l k v r ->
            Node h (mapWithKey f l) k (f k v) (mapWithKey f r)

test "mapWithKey" =
    let m = fromList [(1, 10), (2, 20)]
    let m2 = mapWithKey (fn k v -> k + v) m
    assert (unwrap (lookup 1 m2) == 11)
    assert (unwrap (lookup 2 m2) == 22)


-- | Keep only entries where the predicate returns true.
filter : (k -> v -> Bool) -> Map k v -> Map k v
let filter pred m =
    foldl (fn k v acc ->
        if pred k v then
            insert k v acc
        else
            acc) empty m

test "filter" =
    let m = fromList [(1, 10), (2, 20), (3, 30)]
    let m2 = filter (fn k v -> v > 15) m
    assert (size m2 == 2)
    assert (not (member 1 m2))
    assert (member 2 m2)


-- # Set operations


-- | Left-biased merge of two maps (m1 wins on key collision).
--
--     union (fromList [(1, 10)]) (fromList [(1, 99), (2, 20)]) == fromList [(1, 10), (2, 20)]
--
union : Map k v -> Map k v -> Map k v
let union m1 m2 =
    foldl (fn k v acc -> insert k v acc) m2 m1

test "union" =
    let m1 = fromList [(1, 10), (2, 20)]
    let m2 = fromList [(2, 99), (3, 30)]
    let m3 = union m1 m2
    assert (size m3 == 3)
    assert (unwrap (lookup 1 m3) == 10)
    assert (unwrap (lookup 2 m3) == 20)
    assert (unwrap (lookup 3 m3) == 30)


-- | Merge two maps with a conflict resolver.
--
--     unionWith (fn a b -> a + b) (fromList [(1, 10)]) (fromList [(1, 20), (2, 30)])
--
unionWith : (v -> v -> v) -> Map k v -> Map k v -> Map k v
let unionWith f m1 m2 =
    foldl (fn k v acc ->
        case lookup k acc of
            Just existing ->
                insert k (f v existing) acc
            Nothing ->
                insert k v acc) m2 m1

test "unionWith" =
    let m1 = fromList [(1, 10), (2, 20)]
    let m2 = fromList [(2, 30), (3, 40)]
    let m3 = unionWith (fn a b -> a + b) m1 m2
    assert (size m3 == 3)
    assert (unwrap (lookup 2 m3) == 50)


-- | Keep only keys present in both maps (values from m1).
--
--     intersect (fromList [(1, 10), (2, 20)]) (fromList [(2, 99), (3, 30)]) == fromList [(2, 20)]
--
intersect : Map k v -> Map k v -> Map k v
let intersect m1 m2 =
    filter (fn k v -> member k m2) m1

test "intersect" =
    let m1 = fromList [(1, 10), (2, 20), (3, 30)]
    let m2 = fromList [(2, 99), (3, 88)]
    let m3 = intersect m1 m2
    assert (size m3 == 2)
    assert (unwrap (lookup 2 m3) == 20)
    assert (not (member 1 m3))


-- | Intersect with a value combiner.
--
--     intersectWith (fn a b -> a + b) (fromList [(1, 10), (2, 20)]) (fromList [(2, 30), (3, 40)])
--
intersectWith : (v -> v -> v) -> Map k v -> Map k v -> Map k v
let intersectWith f m1 m2 =
    foldl (fn k v acc ->
        case lookup k m2 of
            Just v2 ->
                insert k (f v v2) acc
            Nothing ->
                acc) empty m1

test "intersectWith" =
    let m1 = fromList [(1, 10), (2, 20)]
    let m2 = fromList [(2, 30), (3, 40)]
    let m3 = intersectWith (fn a b -> a + b) m1 m2
    assert (size m3 == 1)
    assert (unwrap (lookup 2 m3) == 50)


-- | Keys in m1 but not in m2.
--
--     difference (fromList [(1, 10), (2, 20), (3, 30)]) (fromList [(2, 99)]) == fromList [(1, 10), (3, 30)]
--
difference : Map k v -> Map k v -> Map k v
let difference m1 m2 =
    filter (fn k v -> not (member k m2)) m1

test "difference" =
    let m1 = fromList [(1, 10), (2, 20), (3, 30)]
    let m2 = fromList [(2, 99)]
    let m3 = difference m1 m2
    assert (size m3 == 2)
    assert (member 1 m3)
    assert (not (member 2 m3))
    assert (member 3 m3)


-- # Min/Max


-- | Return the smallest key-value pair, or Nothing for empty maps.
--
--     findMin (fromList [(3, 30), (1, 10), (2, 20)]) == Just (1, 10)
--
findMin : Map k v -> Maybe (k, v)
let rec findMin m =
    case m of
        Empty ->
            Nothing
        Node _ (Empty) k v _ ->
            Just (k, v)
        Node _ l _ _ _ ->
            findMin l


-- | Return the largest key-value pair, or Nothing for empty maps.
--
--     findMax (fromList [(3, 30), (1, 10), (2, 20)]) == Just (3, 30)
--
findMax : Map k v -> Maybe (k, v)
let rec findMax m =
    case m of
        Empty ->
            Nothing
        Node _ _ k v (Empty) ->
            Just (k, v)
        Node _ _ _ _ r ->
            findMax r

test "findMin and findMax" =
    let m = fromList [(3, 30), (1, 10), (2, 20)]
    assert (findMin m == Just (1, 10))
    assert (findMax m == Just (3, 30))
    assert (findMin empty == Nothing)
    assert (findMax empty == Nothing)


-- # Predicates


-- | Check if any key-value pair satisfies the predicate.
--
--     any (fn k v -> v > 20) (fromList [(1, 10), (2, 30)]) == true
--
any : (k -> v -> Bool) -> Map k v -> Bool
let any pred m =
    foldl (fn k v acc -> acc || pred k v) false m


-- | Check if all key-value pairs satisfy the predicate.
--
--     all (fn k v -> v > 0) (fromList [(1, 10), (2, 20)]) == true
--
all : (k -> v -> Bool) -> Map k v -> Bool
let all pred m =
    foldl (fn k v acc -> acc && pred k v) true m

test "any and all" =
    let m = fromList [(1, 10), (2, 30)]
    assert (any (fn k v -> v > 20) m)
    assert (not (any (fn k v -> v > 100) m))
    assert (all (fn k v -> v > 0) m)
    assert (not (all (fn k v -> v > 20) m))
