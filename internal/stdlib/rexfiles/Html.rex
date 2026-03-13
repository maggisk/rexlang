-- Std:Html — Virtual DOM types and element helpers
--
-- Html msg is a tree of virtual DOM nodes. The msg type parameter
-- is the message type for event handlers.

import Std:List (map, intersperse)
import Std:String (join, replace)


-- # Types


export type Html msg
    = Element String [Attribute msg] [Html msg]
    | Text String
    | Mounted String Component

export type Attribute msg
    = Attr String String
    | On String msg
    | OnInput (String -> msg)
    | BoolAttr String Bool

export opaque type Component = MkComponent (() -> String)


-- # Elements


export
div : [Attribute msg] -> [Html msg] -> Html msg
div = Element "div"

export
span : [Attribute msg] -> [Html msg] -> Html msg
span = Element "span"

export
button : [Attribute msg] -> [Html msg] -> Html msg
button = Element "button"

export
p : [Attribute msg] -> [Html msg] -> Html msg
p = Element "p"

export
h1 : [Attribute msg] -> [Html msg] -> Html msg
h1 = Element "h1"

export
h2 : [Attribute msg] -> [Html msg] -> Html msg
h2 = Element "h2"

export
h3 : [Attribute msg] -> [Html msg] -> Html msg
h3 = Element "h3"

export
ul : [Attribute msg] -> [Html msg] -> Html msg
ul = Element "ul"

export
ol : [Attribute msg] -> [Html msg] -> Html msg
ol = Element "ol"

export
li : [Attribute msg] -> [Html msg] -> Html msg
li = Element "li"

export
a : [Attribute msg] -> [Html msg] -> Html msg
a = Element "a"

export
img : [Attribute msg] -> Html msg
img attrs = Element "img" attrs []

export
input : [Attribute msg] -> Html msg
input attrs = Element "input" attrs []

export
form : [Attribute msg] -> [Html msg] -> Html msg
form = Element "form"

export
label : [Attribute msg] -> [Html msg] -> Html msg
label = Element "label"

export
section : [Attribute msg] -> [Html msg] -> Html msg
section = Element "section"

export
header : [Attribute msg] -> [Html msg] -> Html msg
header = Element "header"

export
footer : [Attribute msg] -> [Html msg] -> Html msg
footer = Element "footer"

export
nav : [Attribute msg] -> [Html msg] -> Html msg
nav = Element "nav"

export
textarea : [Attribute msg] -> [Html msg] -> Html msg
textarea = Element "textarea"


-- # Content


export
text : String -> Html msg
text = Text


-- # Attributes


export
class : String -> Attribute msg
class name = Attr "class" name

export
id : String -> Attribute msg
id name = Attr "id" name

export
href : String -> Attribute msg
href url = Attr "href" url

export
src : String -> Attribute msg
src url = Attr "src" url

export
value : String -> Attribute msg
value v = Attr "value" v

export
placeholder : String -> Attribute msg
placeholder v = Attr "placeholder" v

export
type_ : String -> Attribute msg
type_ v = Attr "type" v

export
disabled : Bool -> Attribute msg
disabled b = BoolAttr "disabled" b

export
checked : Bool -> Attribute msg
checked b = BoolAttr "checked" b


-- # Events


export
onClick : msg -> Attribute msg
onClick msg = On "click" msg

export
onSubmit : msg -> Attribute msg
onSubmit msg = On "submit" msg

export
onInput : (String -> msg) -> Attribute msg
onInput f = OnInput f


-- # Components


-- | Create a component from a render function.
-- The render function is a closure that captures init/update/view
-- with their concrete types. Component is opaque — the types are erased.
export
makeComponent : (() -> String) -> Component
makeComponent render = MkComponent render

export
mount : String -> Component -> Html msg
mount key comp = Mounted key comp


-- # Browser mount
--
-- In the browser, the JS codegen intercepts this call and uses the VDOM
-- runtime to create real DOM elements with event handlers.
-- In the interpreter, this is a no-op (returns unit).

export
browserMount : String -> model -> (msg -> model -> model) -> (model -> Html msg) -> ()
browserMount _ _ _ _ = ()


-- # Render to string (for testing / SSR)


export
renderToString : Html msg -> String
renderToString node =
    match node
        when Text s ->
            escapeHtml s
        when Element tag attrs children ->
            let attrStr = renderAttrs attrs
            in let childStr = children |> map renderToString |> join ""
            in if tag == "input" || tag == "img" then
                "<" ++ tag ++ attrStr ++ " />"
            else
                "<" ++ tag ++ attrStr ++ ">" ++ childStr ++ "</" ++ tag ++ ">"
        when Mounted key comp ->
            match comp
                when MkComponent render ->
                    render ()


renderAttrs : [Attribute msg] -> String
renderAttrs attrs =
    let
        renderAttr attr =
            match attr
                when Attr name val ->
                    " " ++ name ++ "=\"" ++ escapeHtml val ++ "\""
                when On name _ ->
                    ""
                when OnInput _ ->
                    ""
                when BoolAttr name val ->
                    if val then " " ++ name else ""
    in
    attrs |> map renderAttr |> join ""


escapeHtml : String -> String
escapeHtml s =
    s
    |> replace "&" "&amp;"
    |> replace "<" "&lt;"
    |> replace ">" "&gt;"
    |> replace "\"" "&quot;"


-- # Tests


test "text node" =
    assert (renderToString (text "hello") == "hello")

test "text escapes html" =
    assert (renderToString (text "<script>") == "&lt;script&gt;")

test "simple element" =
    assert (renderToString (div [] [text "hi"]) == "<div>hi</div>")

test "element with class" =
    assert (renderToString (div [class "foo"] [text "hi"]) == "<div class=\"foo\">hi</div>")

test "nested elements" =
    let html = ul [] [li [] [text "one"], li [] [text "two"]]
    in assert (renderToString html == "<ul><li>one</li><li>two</li></ul>")

test "self-closing elements" =
    assert (renderToString (input [type_ "text", value "hello"]) == "<input type=\"text\" value=\"hello\" />")

test "bool attribute true" =
    assert (renderToString (input [disabled true]) == "<input disabled />")

test "bool attribute false" =
    assert (renderToString (input [disabled false]) == "<input />")

test "events not rendered" =
    assert (renderToString (button [onClick "msg", class "btn"] [text "go"]) == "<button class=\"btn\">go</button>")
