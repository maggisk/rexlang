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


-- ## Parameterized instances

impl Show (List a) where
    show xs =
        let rec showItems items first =
            match items
                when [] ->
                    ""
                when [h|t] ->
                    let prefix = if first then "" else ", "
                    in prefix ++ show h ++ showItems t false
        in "[" ++ showItems xs true ++ "]"

impl Show (a, b) where
    show pair =
        let (x, y) = pair
        in "(" ++ show x ++ ", " ++ show y ++ ")"

impl Show () where
    show _ = "()"

impl Eq (List a) where
    eq xs ys =
        match xs
            when [] ->
                match ys
                    when [] ->
                        true
                    when _ ->
                        false
            when [x|xrest] ->
                match ys
                    when [] ->
                        false
                    when [y|yrest] ->
                        eq x y && eq xrest yrest

impl Eq (a, b) where
    eq x y =
        let (a1, b1) = x
        in let (a2, b2) = y
        in eq a1 a2 && eq b1 b2

impl Ord (List a) where
    compare xs ys =
        match xs
            when [] ->
                match ys
                    when [] ->
                        EQ
                    when _ ->
                        LT
            when [x|xrest] ->
                match ys
                    when [] ->
                        GT
                    when [y|yrest] ->
                        match compare x y
                            when EQ ->
                                compare xrest yrest
                            when result ->
                                result

impl Ord (a, b) where
    compare x y =
        let (a1, b1) = x
        in let (a2, b2) = y
        in
        match compare a1 a2
            when EQ ->
                compare b1 b2
            when result ->
                result
