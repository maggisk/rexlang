-- # Prelude — loaded automatically before user code


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


-- ## Show trait

trait Show a where
    show : a -> String

impl Show Int where
    show n = showInt n

impl Show Float where
    show n = showFloat n

impl Show Bool where
    show b =
        if b then
            "true"
        else
            "false"

impl Show String where
    show s = s
