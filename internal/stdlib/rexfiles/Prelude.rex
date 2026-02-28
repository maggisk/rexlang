-- # Prelude — loaded automatically before user code


-- ## Maybe type

type Maybe a = Nothing | Just a


-- ## Ordering type

type Ordering = LT | EQ | GT


-- ## Eq trait

trait Eq a where
    eq : a -> a -> Bool

impl Eq Int where
    eq x y = x == y

impl Eq Float where
    eq x y = x == y

impl Eq String where
    eq x y = x == y

impl Eq Bool where
    eq x y = x == y


-- ## Ord trait

trait Ord a where
    compare : a -> a -> Ordering

impl Ord Int where
    compare x y =
        if x < y then
            LT
        else if x == y then
            EQ
        else
            GT

impl Ord Float where
    compare x y =
        if x < y then
            LT
        else if x == y then
            EQ
        else
            GT

impl Ord String where
    compare x y =
        if x < y then
            LT
        else if x == y then
            EQ
        else
            GT

impl Ord Bool where
    compare x y =
        if x < y then
            LT
        else if x == y then
            EQ
        else
            GT
