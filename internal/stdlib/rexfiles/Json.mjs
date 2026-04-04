// Json.mjs — JS companion for Std:Json

function jsToRexJson(v) {
    if (v === null) return { $tag: "JNull", $type: "Json" };
    if (typeof v === "boolean") return { $tag: "JBool", $type: "Json", _0: v };
    if (typeof v === "number") return { $tag: "JNum", $type: "Json", _0: v };
    if (typeof v === "string") return { $tag: "JStr", $type: "Json", _0: v };
    if (Array.isArray(v)) {
        let lst = null;
        for (let i = v.length - 1; i >= 0; i--) {
            lst = { $tag: "Cons", head: jsToRexJson(v[i]), tail: lst };
        }
        return { $tag: "JArr", $type: "Json", _0: lst };
    }
    if (typeof v === "object") {
        let lst = null;
        const keys = Object.keys(v).reverse();
        for (const k of keys) {
            lst = { $tag: "Cons", head: [k, jsToRexJson(v[k])], tail: lst };
        }
        return { $tag: "JObj", $type: "Json", _0: lst };
    }
    return { $tag: "JNull", $type: "Json" };
}

export function jsonParse(s) {
    try {
        const v = JSON.parse(s);
        return { $tag: "Ok", $type: "Result", _0: jsToRexJson(v) };
    } catch (e) {
        return { $tag: "Err", $type: "Result", _0: e.message };
    }
}
