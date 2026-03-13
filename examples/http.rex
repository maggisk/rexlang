import Std:IO (println)
import Std:Result (Ok, Err)
import Std:Http.Server (serve, ok, html, json, text, segments, getQuery, Request, Response)
import Std:String (split)

handle : Request -> Response
handle req =
    match (req.method, segments req.path)
        when ("GET", []) ->
            html 200 "<h1>Welcome to Rex</h1><p>Try /hello/world or /api/users</p>"
        when ("GET", ["hello", name]) ->
            html 200 "<h1>Hello, ${name}!</h1>"
        when ("GET", ["api", "users"]) ->
            json 200 "[{\"name\": \"Alice\"}, {\"name\": \"Bob\"}]"
        when ("GET", ["api", "users", id]) ->
            json 200 "{\"id\": \"${id}\"}"
        when ("POST", ["api", "users"]) ->
            match req
                when Request { body = b } ->
                    json 201 b
        when _ ->
            text 404 "not found"

export
main _ =
    match serve 3000 handle
        when Err e ->
            let _ = println ("Error: " ++ e)
            in 1
        when Ok _ ->
            0
