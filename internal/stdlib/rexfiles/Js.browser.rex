-- # Std:Js — Generic JavaScript FFI primitives (browser-only)
--
-- Provides low-level interop with JavaScript values via an opaque JsRef type.
-- In compiled JS output, JsRef is just a raw JS value (no handle table).
-- The `error` bodies are placeholders — the JS codegen replaces all calls
-- with inline JS.

import Std:Result (Ok, Err)


-- # JsRef type

-- | Opaque reference to a JavaScript value.
export opaque type JsRef = | JsRef


-- # Global access

-- | Look up a global variable by name (e.g., "document", "console").
export
jsGlobal : String -> Result JsRef String
jsGlobal _ = error "browser-only builtin"


-- # Property access

-- | Get a property from a JS object by name.
export
jsGet : String -> JsRef -> Result JsRef String
jsGet _ _ = error "browser-only builtin"

-- | Set a property on a JS object.
export
jsSet : String -> JsRef -> JsRef -> Result () String
jsSet _ _ _ = error "browser-only builtin"


-- # Method calls and constructors

-- | Call a method on a JS object with arguments.
export
jsCall : String -> List JsRef -> JsRef -> Result JsRef String
jsCall _ _ _ = error "browser-only builtin"

-- | Invoke a global constructor with `new` (e.g., `jsNew "Date" []`).
export
jsNew : String -> List JsRef -> Result JsRef String
jsNew _ _ = error "browser-only builtin"


-- # Callbacks

-- | Wrap a Rex function as a JS callback.
-- The callback receives the first JS argument (or null if none).
export
jsCallback : (JsRef -> msg) -> JsRef
jsCallback _ = error "browser-only builtin"


-- # Conversion to JsRef

-- | Convert a Rex String to a JsRef.
export
jsFromString : String -> JsRef
jsFromString _ = error "browser-only builtin"

-- | Convert a Rex Int to a JsRef.
export
jsFromInt : Int -> JsRef
jsFromInt _ = error "browser-only builtin"

-- | Convert a Rex Float to a JsRef.
export
jsFromFloat : Float -> JsRef
jsFromFloat _ = error "browser-only builtin"

-- | Convert a Rex Bool to a JsRef.
export
jsFromBool : Bool -> JsRef
jsFromBool _ = error "browser-only builtin"


-- # Conversion from JsRef

-- | Extract a String from a JsRef.
export
jsToString : JsRef -> Result String String
jsToString _ = error "browser-only builtin"

-- | Extract an Int from a JsRef.
export
jsToInt : JsRef -> Result Int String
jsToInt _ = error "browser-only builtin"

-- | Extract a Float from a JsRef.
export
jsToFloat : JsRef -> Result Float String
jsToFloat _ = error "browser-only builtin"

-- | Extract a Bool from a JsRef.
export
jsToBool : JsRef -> Result Bool String
jsToBool _ = error "browser-only builtin"


-- # Constants

-- | JavaScript `null`.
export
jsNull : JsRef
jsNull = error "browser-only builtin"
