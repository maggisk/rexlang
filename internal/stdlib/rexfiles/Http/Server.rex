-- Http.Server — HTTP server built on Go's net/http
--
-- One Go builtin (httpServe) handles listening and HTTP parsing.
-- Everything else is pure Rex.

import Std:List (map, filter, foldl)
import Std:String (split, indexOf, substring, length)
import Std:Maybe (Just, Nothing)


-- # Types


export type Request =
    { method  : String
    , path    : String
    , headers : [(String, String)]
    , body    : String
    , query   : [(String, String)]
    }

export type Response =
    { status  : Int
    , headers : [(String, String)]
    , body    : String
    }


-- # Server


export external httpServe : Int -> (Request -> Response) -> Result () String

export
serve : Int -> (Request -> Response) -> Result () String
serve = httpServe


-- # Response helpers


export
ok : String -> Response
ok body =
    Response { status = 200, headers = [("Content-Type", "text/plain")], body = body }

test "ok response" =
    let r = ok "hello"
    in assert (r.status == 200)


export
html : Int -> String -> Response
html code body =
    Response { status = code, headers = [("Content-Type", "text/html; charset=utf-8")], body = body }

test "html response" =
    let r = html 200 "<h1>hi</h1>"
    in assert (r.status == 200)


export
json : Int -> String -> Response
json code body =
    Response { status = code, headers = [("Content-Type", "application/json")], body = body }

test "json response" =
    let r = json 201 "{}"
    in assert (r.status == 201)


export
text : Int -> String -> Response
text code body =
    Response { status = code, headers = [("Content-Type", "text/plain")], body = body }

test "text response" =
    let r = text 404 "not found"
    in assert (r.status == 404)


export
redirect : Int -> String -> Response
redirect code url =
    Response { status = code, headers = [("Location", url)], body = "" }

test "redirect" =
    let r = redirect 302 "/login"
    in assert (r.status == 302)


export
withHeader : String -> String -> Response -> Response
withHeader name value resp =
    match resp
        when Response { headers = hs } ->
            { resp | headers = (name, value) :: hs }

test "withHeader" =
    let r = ok "hello" |> withHeader "X-Custom" "value"
    in assert (r.status == 200)


-- # Request helpers


findPair : String -> [(String, String)] -> Maybe String
findPair key pairs =
    match pairs
        when [] ->
            Nothing
        when [(k, v) | rest] ->
            if k == key then
                Just v
            else
                findPair key rest


export
getHeader : String -> Request -> Maybe String
getHeader name req =
    match req
        when Request { headers = hs } ->
            findPair name hs

test "getHeader found" =
    let req = Request { method = "GET", path = "/", headers = [("Host", "example.com")], body = "", query = [] }
    in assert (getHeader "Host" req == Just "example.com")

test "getHeader not found" =
    let req = Request { method = "GET", path = "/", headers = [], body = "", query = [] }
    in assert (getHeader "Host" req == Nothing)


export
getQuery : String -> Request -> Maybe String
getQuery name req =
    match req
        when Request { query = qs } ->
            findPair name qs

test "getQuery found" =
    let req = Request { method = "GET", path = "/", headers = [], body = "", query = [("page", "2")] }
    in assert (getQuery "page" req == Just "2")

test "getQuery not found" =
    let req = Request { method = "GET", path = "/", headers = [], body = "", query = [] }
    in assert (getQuery "page" req == Nothing)


-- # Path helpers


export
segments : String -> [String]
segments path =
    path |> split "/" |> filter (\s -> s != "")

test "segments" =
    assert (segments "/users/42/posts" == ["users", "42", "posts"])

test "segments root" =
    assert (segments "/" == [])

test "segments no leading slash" =
    assert (segments "users" == ["users"])
