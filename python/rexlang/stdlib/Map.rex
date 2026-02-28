export empty, singleton, fromList, lookup, member, size, isEmpty, insert, remove, update, map, filter, foldl, foldr, toList, keys, values


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
    let lh = height l in
    let rh = height r in
    let h = if lh > rh then
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


-- # Create


-- | An empty map.
let empty = Empty


-- | Create a map with a single key-value pair.
let singleton k v = Node 1 Empty k v Empty


-- # Modify


-- | Insert a key-value pair, replacing any existing value for the key.
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


-- | Update the value at a key by applying a function. No-op if key absent.
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


-- # Query


-- | Look up a key, returning Just value or Nothing.
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


-- | Count the number of entries in the map.
let rec size m =
    case m of
        Empty ->
            0
        Node _ l _ _ r ->
            1 + size l + size r


-- | Check if the map is empty.
let isEmpty m =
    case m of
        Empty ->
            true
        Node _ _ _ _ _ ->
            false


-- # Fold


-- | Fold over key-value pairs from smallest to largest key.
let rec foldl f acc m =
    case m of
        Empty ->
            acc
        Node _ l k v r ->
            let acc1 = foldl f acc l in
            let acc2 = f k v acc1 in
            foldl f acc2 r


-- | Fold over key-value pairs from largest to smallest key.
let rec foldr f acc m =
    case m of
        Empty ->
            acc
        Node _ l k v r ->
            let acc1 = foldr f acc r in
            let acc2 = f k v acc1 in
            foldr f acc2 l


-- # Convert


-- | Convert to a sorted list of (key, value) pairs.
let toList m =
    foldr (fun k v acc -> (k, v) :: acc) [] m


-- | Get all keys in sorted order.
let keys m =
    foldr (fun k v acc -> k :: acc) [] m


-- | Get all values in key order.
let values m =
    foldr (fun k v acc -> v :: acc) [] m


-- | Build a map from a list of (key, value) pairs.
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


-- # Transform


-- | Apply a function to every value in the map.
let rec map f m =
    case m of
        Empty ->
            Empty
        Node h l k v r ->
            Node h (map f l) k (f v) (map f r)


-- | Keep only entries where the predicate returns true.
let filter pred m =
    foldl (fun k v acc ->
        if pred k v then
            insert k v acc
        else
            acc) empty m
