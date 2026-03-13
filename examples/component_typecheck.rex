-- Test: can Rex typecheck AND run the Component pattern?
-- The runtime needs to store init/update/view and call them later.

-- We can't store polymorphic functions in a typed ADT, so the
-- "runtime" needs to be a closure that captures the typed functions.

-- A Component wraps a "render" function: () -> String
-- The render function closes over the spec and manages state internally.
export opaque type Component = MkComponent (() -> String)

type Spec a b = { init : a, update : b -> a -> a, view : a -> String }

makeComponent : Spec a b -> Component
makeComponent spec =
    -- The closure captures spec with its concrete types
    MkComponent (\_ -> spec.view spec.init)

-- "Run" a component: extract and call the render function
render : Component -> String
render comp =
    match comp
        when MkComponent f -> f ()


-- Counter
type CounterMsg = Inc | Dec

myCounter : Component
myCounter =
    makeComponent (Spec
        { init = 0
        , update = \msg model ->
            match msg
                when Inc -> model + 1
                when Dec -> model - 1
        , view = \model -> "count: ${show model}"
        })


-- Greeter
type GreetMsg = SetName String

myGreeter : Component
myGreeter =
    makeComponent (Spec
        { init = "world"
        , update = \msg model ->
            match msg
                when SetName name -> name
        , view = \model -> "hello ${model}"
        })


-- Parameterized
makeCounter : Int -> Component
makeCounter initial =
    makeComponent (Spec
        { init = initial
        , update = \msg model ->
            match msg
                when Inc -> model + 1
                when Dec -> model - 1
        , view = \model -> "count: ${show model}"
        })


test "render counter" =
    assert (render myCounter == "count: 0")

test "render greeter" =
    assert (render myGreeter == "hello world")

test "render parameterized" =
    assert (render (makeCounter 42) == "count: 42")

test "render list of mixed components" =
    let components = [myCounter, myGreeter, makeCounter 99]
    in let results = components |> map render
    in assert results == ["count: 0", "hello world", "count: 99"]


-- Helper (inline since no import)
map f lst =
    match lst
        when [] -> []
        when [h|t] -> f h :: map f t
