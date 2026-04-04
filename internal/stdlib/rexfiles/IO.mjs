// IO.mjs — JS companion for Std:IO (browser target)

export function print(v) {
    if (typeof v === "string") {
        console.log(v);
    } else {
        console.log(v === null ? "()" : String(v));
    }
    return v;
}

export function println(v) {
    if (typeof v === "string") {
        console.log(v);
    } else {
        console.log(v === null ? "()" : String(v));
    }
    return v;
}

export function readLine(_prompt) {
    return "";
}

export function readFile(_path) {
    return { $tag: "Err", $type: "Result", _0: "readFile not available in browser" };
}

export function writeFile(_path, _content) {
    return { $tag: "Err", $type: "Result", _0: "writeFile not available in browser" };
}

export function appendFile(_path, _content) {
    return { $tag: "Err", $type: "Result", _0: "appendFile not available in browser" };
}

export function fileExists(_path) {
    return false;
}

export function listDir(_path) {
    return { $tag: "Err", $type: "Result", _0: "listDir not available in browser" };
}
