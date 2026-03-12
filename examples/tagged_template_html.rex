-- Tagged templates: building safe HTML with automatic escaping
--
-- Tag functions receive [String] (the literal fragments) and [a] (the
-- interpolated values). This lets you process values before splicing
-- them in — e.g., escaping HTML entities to prevent XSS.

import Std:List (interleave, map, indexedMap)
import Std:String (replace, join, toString)


-- # HTML tagged template

-- | HtmlValue wraps interpolated values so the tag function knows what to do.
type HtmlValue = Text String | Raw String

text : String -> HtmlValue
text s = Text s

raw : String -> HtmlValue
raw s = Raw s


-- | Escape HTML entities in a string.
escapeHtml : String -> String
escapeHtml s =
    s
    |> replace "&" "&amp;"
    |> replace "<" "&lt;"
    |> replace ">" "&gt;"
    |> replace "\"" "&quot;"

-- | Render an HtmlValue to a string.
renderValue : HtmlValue -> String
renderValue v =
    match v
        when Text s ->
            escapeHtml s
        when Raw s ->
            s

-- | The html tag function: escapes Text values, passes Raw values through.
html : [String] -> [HtmlValue] -> String
html strings values =
    interleave strings (values |> map renderValue) |> join ""


-- # Examples

userName : String
userName = "Alice <script>alert('xss')</script>"

userBio : String
userBio = "Likes <b>bold</b> text & ampersands"


test "html escapes text values" =
    let result = html`<span>${text userName}</span>`
    in assert result == "<span>Alice &lt;script&gt;alert('xss')&lt;/script&gt;</span>"

test "html passes raw values through" =
    let result = html`<div>${raw "<strong>bold</strong>"}</div>`
    in assert result == "<div><strong>bold</strong></div>"

test "html with multiple values" =
    let
        title = text "Hello & Goodbye"
        body = text userBio
    in
    let result = html`<h1>${title}</h1><p>${body}</p>`
    in assert result == "<h1>Hello &amp; Goodbye</h1><p>Likes &lt;b&gt;bold&lt;/b&gt; text &amp; ampersands</p>"

test "html with no interpolations" =
    let result = html`<br />`
    in assert result == "<br />"


-- # SQL tagged template (parameterized queries)

-- | SqlParam wraps values for parameterized SQL queries.
type SqlParam = SInt Int | SStr String | SFloat Float | SBool Bool

-- | SqlQuery holds the query string with $1, $2, ... placeholders
-- | and the list of parameter values.
type SqlQuery = { query : String, params : [SqlParam] }


-- | The sql tag function: replaces interpolations with $1, $2, ... placeholders.
sql : [String] -> [SqlParam] -> SqlQuery
sql strings params =
    let placeholders = params |> indexedMap (\i _ -> "$" ++ toString (i + 1))
    in SqlQuery { query = interleave strings placeholders |> join "", params = params }


test "sql parameterized query" =
    let q = sql`SELECT * FROM users WHERE name = ${SStr "Alice"} AND age > ${SInt 18}`
    in
    let
        _ = assert q.query == "SELECT * FROM users WHERE name = $1 AND age > $2"
    in assert q.params == [SStr "Alice", SInt 18]

test "sql query with no params" =
    let q = sql`SELECT count(*) FROM users`
    in
    let
        _ = assert q.query == "SELECT count(*) FROM users"
    in assert q.params == []
