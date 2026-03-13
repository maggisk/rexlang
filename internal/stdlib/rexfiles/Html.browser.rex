-- # Std:Html browser overlay — VDOM diffing + TEA runtime
--
-- When --target=browser, this file is concatenated after Html.rex.
-- The second browserMount definition shadows the no-op stub.

import Std:Js (jsGlobal, jsGet, jsSet, jsCall, jsCallback,
               jsFromString, jsFromInt, jsFromBool, jsToString, jsToInt)
import Std:Result (Ok, Err)
import Std:Process (spawn, self, send, receive)


-- # Helpers


unwrap : Result a String -> a
unwrap r =
    match r
        when Ok v ->
            v
        when Err msg ->
            error msg

discard : a -> ()
discard _ = ()


-- # DOM primitives


document : JsRef
document = jsGlobal "document" |> unwrap

getElementById : String -> JsRef
getElementById eid =
    jsCall "getElementById" [jsFromString eid] document |> unwrap

createElement : String -> JsRef
createElement tag =
    jsCall "createElement" [jsFromString tag] document |> unwrap

createTextNode : String -> JsRef
createTextNode s =
    jsCall "createTextNode" [jsFromString s] document |> unwrap

setAttribute : JsRef -> String -> String -> ()
setAttribute el name val =
    jsCall "setAttribute" [jsFromString name, jsFromString val] el |> unwrap |> discard

removeAttribute : JsRef -> String -> ()
removeAttribute el name =
    jsCall "removeAttribute" [jsFromString name] el |> unwrap |> discard

appendChild : JsRef -> JsRef -> ()
appendChild parent child =
    jsCall "appendChild" [child] parent |> unwrap |> discard

removeChild : JsRef -> JsRef -> ()
removeChild parent child =
    jsCall "removeChild" [child] parent |> unwrap |> discard

replaceChild : JsRef -> JsRef -> JsRef -> ()
replaceChild parent newChild oldChild =
    jsCall "replaceChild" [newChild, oldChild] parent |> unwrap |> discard

addEventListener : JsRef -> String -> JsRef -> ()
addEventListener el eventName cb =
    jsCall "addEventListener" [jsFromString eventName, cb] el |> unwrap |> discard

removeEventListener : JsRef -> String -> JsRef -> ()
removeEventListener el eventName cb =
    jsCall "removeEventListener" [jsFromString eventName, cb] el |> unwrap |> discard

getChildNode : JsRef -> Int -> JsRef
getChildNode parent index =
    let nodes = jsGet "childNodes" parent |> unwrap
    in jsCall "item" [jsFromInt index] nodes |> unwrap

childCount : JsRef -> Int
childCount parent =
    let nodes = jsGet "childNodes" parent |> unwrap
    in jsGet "length" nodes |> unwrap |> jsToInt |> unwrap

setTextContent : JsRef -> String -> ()
setTextContent el s =
    jsSet "textContent" el (jsFromString s) |> unwrap |> discard

setProperty : JsRef -> String -> String -> ()
setProperty el prop val =
    jsSet prop el (jsFromString val) |> unwrap |> discard

preventDefault : JsRef -> ()
preventDefault event =
    jsCall "preventDefault" [] event |> unwrap |> discard

setInnerHTML : JsRef -> String -> ()
setInnerHTML el html =
    jsSet "innerHTML" el (jsFromString html) |> unwrap |> discard


-- # VDOM -> DOM


vdomCreateDom : Html msg -> (msg -> ()) -> JsRef
vdomCreateDom node dispatch =
    match node
        when Text s ->
            createTextNode s
        when Element tag attrs children ->
            createElementDom tag attrs children dispatch
        when Mounted key comp ->
            createMountedDom comp


createElementDom : String -> [Attribute msg] -> [Html msg] -> (msg -> ()) -> JsRef
createElementDom tag attrs children dispatch =
    let
        el = createElement tag
        _ = applyAttrs el attrs dispatch
        _ = applyChildren el children dispatch
    in el


createMountedDom : Component -> JsRef
createMountedDom comp =
    match comp
        when MkComponent render ->
            let
                wrapper = createElement "span"
                _ = setInnerHTML wrapper (render ())
            in wrapper


applyAttrs : JsRef -> [Attribute msg] -> (msg -> ()) -> ()
applyAttrs el attrs dispatch =
    match attrs
        when [] ->
            ()
        when [a|rest] ->
            let _ = applyAttr el a dispatch
            in applyAttrs el rest dispatch


applyAttr : JsRef -> Attribute msg -> (msg -> ()) -> ()
applyAttr el attr dispatch =
    match attr
        when Attr name val ->
            if name == "value" then
                setProperty el "value" val
            else
                setAttribute el name val
        when On eventName msg ->
            applyOnEvent el eventName msg dispatch
        when OnInput fn ->
            applyOnInput el fn dispatch
        when BoolAttr name val ->
            if val then
                setAttribute el name ""
            else
                ()


applyOnEvent : JsRef -> String -> msg -> (msg -> ()) -> ()
applyOnEvent el eventName msg dispatch =
    let
        cb = jsCallback (\event -> let _ = preventDefault event in let _ = dispatch msg in ())
        _ = addEventListener el eventName cb
        _ = jsSet ("__cb_" ++ eventName) el cb |> unwrap
    in ()


applyOnInput : JsRef -> (String -> msg) -> (msg -> ()) -> ()
applyOnInput el fn dispatch =
    let
        cb = jsCallback (\event -> let target = jsGet "target" event |> unwrap in let val = jsGet "value" target |> unwrap |> jsToString |> unwrap in let _ = dispatch (fn val) in ())
        _ = addEventListener el "input" cb
        _ = jsSet "__cb_input" el cb |> unwrap
    in ()


applyChildren : JsRef -> [Html msg] -> (msg -> ()) -> ()
applyChildren parent children dispatch =
    match children
        when [] ->
            ()
        when [c|rest] ->
            let _ = appendChild parent (vdomCreateDom c dispatch)
            in applyChildren parent rest dispatch


-- # Diff + Patch


vdomPatch : JsRef -> Html msg -> Html msg -> (msg -> ()) -> ()
vdomPatch parent old new dispatch =
    patchAt parent old new 0 dispatch


patchAt : JsRef -> Html msg -> Html msg -> Int -> (msg -> ()) -> ()
patchAt parent old new index dispatch =
    let domNode = getChildNode parent index
    in patchNode parent domNode old new dispatch


patchNode : JsRef -> JsRef -> Html msg -> Html msg -> (msg -> ()) -> ()
patchNode parent domNode old new dispatch =
    match (old, new)
        when (Text a, Text b) ->
            if a != b then
                setTextContent domNode b
            else
                ()
        when (Element t1 a1 c1, Element t2 a2 c2) ->
            patchElement parent domNode t1 a1 c1 t2 a2 c2 dispatch
        when (Mounted k1 _, Mounted k2 _) ->
            if k1 == k2 then
                ()
            else
                replaceNode parent domNode new dispatch
        when _ ->
            replaceNode parent domNode new dispatch


patchElement : JsRef -> JsRef -> String -> [Attribute msg] -> [Html msg] -> String -> [Attribute msg] -> [Html msg] -> (msg -> ()) -> ()
patchElement parent domNode t1 a1 c1 t2 a2 c2 dispatch =
    if t1 == t2 then
        let _ = patchAttrs domNode a1 a2 dispatch
        in patchChildren domNode c1 c2 dispatch
    else
        replaceNode parent domNode (Element t2 a2 c2) dispatch


replaceNode : JsRef -> JsRef -> Html msg -> (msg -> ()) -> ()
replaceNode parent oldDom newVdom dispatch =
    let newDom = vdomCreateDom newVdom dispatch
    in replaceChild parent newDom oldDom


patchAttrs : JsRef -> [Attribute msg] -> [Attribute msg] -> (msg -> ()) -> ()
patchAttrs el oldAttrs newAttrs dispatch =
    let _ = removeAttrs el oldAttrs
    in applyAttrs el newAttrs dispatch


removeAttrs : JsRef -> [Attribute msg] -> ()
removeAttrs el attrs =
    match attrs
        when [] ->
            ()
        when [a|rest] ->
            let _ = removeAttr el a
            in removeAttrs el rest


removeAttr : JsRef -> Attribute msg -> ()
removeAttr el attr =
    match attr
        when Attr name _ ->
            if name == "value" then
                ()
            else
                removeAttribute el name
        when On eventName _ ->
            let oldCb = jsGet ("__cb_" ++ eventName) el |> unwrap
            in removeEventListener el eventName oldCb
        when OnInput _ ->
            let oldCb = jsGet "__cb_input" el |> unwrap
            in removeEventListener el "input" oldCb
        when BoolAttr name _ ->
            removeAttribute el name


patchChildren : JsRef -> [Html msg] -> [Html msg] -> (msg -> ()) -> ()
patchChildren parent oldChildren newChildren dispatch =
    patchChildrenAt parent oldChildren newChildren 0 dispatch


patchChildrenAt : JsRef -> [Html msg] -> [Html msg] -> Int -> (msg -> ()) -> ()
patchChildrenAt parent olds news index dispatch =
    match (olds, news)
        when ([], []) ->
            ()
        when ([], [n|rest]) ->
            let _ = appendChild parent (vdomCreateDom n dispatch)
            in patchChildrenAt parent [] rest (index + 1) dispatch
        when ([_|rest], []) ->
            removeLastChild parent rest index dispatch
        when ([o|orest], [n|nrest]) ->
            let _ = patchAt parent o n index dispatch
            in patchChildrenAt parent orest nrest (index + 1) dispatch


removeLastChild : JsRef -> [Html msg] -> Int -> (msg -> ()) -> ()
removeLastChild parent rest index dispatch =
    let
        last = jsGet "lastChild" parent |> unwrap
        _ = removeChild parent last
    in patchChildrenAt parent rest [] index dispatch


-- # TEA Runtime


browserMount : String -> model -> (msg -> model -> model) -> (model -> Html msg) -> ()
browserMount rootId model update view =
    let
        root = getElementById rootId
        vdom = view model
        _ = spawn \_ ->
            let
                me = self
                dispatch = (\msg -> send me msg)
                dom = vdomCreateDom vdom dispatch
                _ = appendChild root dom
            in teaLoop me update view root dispatch model vdom
    in ()


teaLoop : Pid msg -> (msg -> model -> model) -> (model -> Html msg) -> JsRef -> (msg -> ()) -> model -> Html msg -> ()
teaLoop me update view root dispatch state prevVdom =
    let
        msg = receive ()
        newState = update msg state
        newVdom = view newState
        _ = vdomPatch root prevVdom newVdom dispatch
    in teaLoop me update view root dispatch newState newVdom
