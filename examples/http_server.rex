-- http_server.rex — A simple HTTP server with routing
--
-- Usage:  rex examples/http_server.rex
--
-- Starts a server on port 3001 with these routes:
--   GET /              HTML welcome page
--   GET /api/hello     JSON greeting
--   GET /api/users     JSON list of users
--   GET /greet/:name   Personalized HTML greeting
--   POST /api/echo     Echoes the request body as JSON
--   *                  404 not found
--
-- Pattern matching on (method, segments) makes routing a clean DSL.

import Std:IO (println)
import Std:Http.Server (serve, html, json, text, segments, Request, Response)
import Std:Result (Ok, Err)
import Std:Math (toFloat)
import Std:Json (stringify, encodeObj, encodeStr, encodeArr, encodeNum)


-- | Build a JSON user object from name and age.
userJson : String -> Int -> Json
userJson name age =
    encodeObj
        [ ("name", encodeStr name)
        , ("age", encodeNum (toFloat age))
        ]


-- | Route an incoming request by matching on method and path segments.
-- This shows how Rex's pattern matching creates a natural routing DSL.
handle : Request -> Response
handle req =
    match (req.method, segments req.path)
        when ("GET", []) ->
            html 200 """
                <h1>Welcome to Rex</h1>
                <p>A functional language that compiles to native binaries.</p>
                <ul>
                    <li><a href="/api/hello">GET /api/hello</a></li>
                    <li><a href="/api/users">GET /api/users</a></li>
                    <li><a href="/greet/world">GET /greet/:name</a></li>
                </ul>
                """
        when ("GET", ["api", "hello"]) ->
            let body =
                encodeObj
                    [ ("message", encodeStr "Hello from Rex!")
                    , ("status", encodeStr "ok")
                    ]
                |> stringify
            in json 200 body
        when ("GET", ["api", "users"]) ->
            let body =
                encodeArr
                    [ userJson "Alice" 30
                    , userJson "Bob" 25
                    , userJson "Charlie" 35
                    ]
                |> stringify
            in json 200 body
        when ("GET", ["greet", name]) ->
            html 200 "<h1>Hello, ${name}!</h1><p><a href=\"/\">Back</a></p>"
        when ("POST", ["api", "echo"]) ->
            let body =
                encodeObj
                    [ ("echo", encodeStr req.body)
                    , ("method", encodeStr req.method)
                    ]
                |> stringify
            in json 200 body
        when _ ->
            text 404 "Not found"


export
main : [String] -> Int
main _ =
    let _ = println "Starting server on http://localhost:3001"
    in
    match serve 3001 handle
        when Err e ->
            let _ = println "Error: ${e}"
            in 1
        when Ok _ ->
            0
