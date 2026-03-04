import Std:String (length, dedent)


-- Plain multi-line string (first newline after """ is stripped)
let poem = """
Roses are red
Violets are blue
"""


-- Interpolation works inside multi-line strings
let name = "World"
let greeting = """
Hello, ${name}!
Welcome to RexLang.
"""


-- Escapes work as normal
let escaped = """
Tab:\there
Backslash: \\
Dollar: \$
"""


-- Content on same line as opening """ (no newline to strip)
let inline = """same line"""


-- Lone " and "" inside triple-quoted strings are fine
let quotes = """
She said "hello" and "goodbye".
Even "" is ok.
"""


test "plain multi-line" =
    assert poem == "Roses are red\nViolets are blue\n"

test "interpolation" =
    assert greeting == "Hello, World!\nWelcome to RexLang.\n"

test "escapes" =
    assert escaped == "Tab:\there\nBackslash: \\\nDollar: $\n"

test "inline triple-quoted" =
    assert inline == "same line"

test "quotes inside" =
    assert quotes == "She said \"hello\" and \"goodbye\".\nEven \"\" is ok.\n"

test "multi-line string length" =
    assert length poem == 31

test "dedent with multi-line string" =
    let html = dedent """
        <div>
            <p>hello</p>
        </div>
        """
    assert html == "<div>\n    <p>hello</p>\n</div>\n"
