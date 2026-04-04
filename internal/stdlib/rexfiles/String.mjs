// String.mjs — JS companion for Std:String

export function length(s) {
    return [...s].length;
}

export function toUpper(s) {
    return s.toUpperCase();
}

export function toLower(s) {
    return s.toLowerCase();
}

export function trim(s) {
    return s.trim();
}

export function trimLeft(s) {
    return s.trimStart();
}

export function trimRight(s) {
    return s.trimEnd();
}

export function split(sep, s) {
    return s.split(sep).reduceRight((t, h) => ({ $tag: "Cons", head: h, tail: t }), null);
}

export function join(sep, lst) {
    const items = [];
    let cur = lst;
    while (cur !== null && cur.$tag === "Cons") { items.push(cur.head); cur = cur.tail; }
    return items.join(sep);
}

export function toString(v) {
    if (v === null) return "()";
    if (typeof v === "number") return String(v);
    if (typeof v === "string") return v;
    if (typeof v === "boolean") return v ? "true" : "false";
    return String(v);
}

export function contains(sub, s) {
    return s.includes(sub);
}

export function startsWith(pfx, s) {
    return s.startsWith(pfx);
}

export function endsWith(sfx, s) {
    return s.endsWith(sfx);
}

export function charAt(i, s) {
    const chars = [...s];
    return i >= 0 && i < chars.length
        ? { $tag: "Just", _0: chars[i], $type: "Maybe" }
        : { $tag: "Nothing", $type: "Maybe" };
}

export function substring(start, end, s) {
    return [...s].slice(start, end).join("");
}

export function indexOf(sub, s) {
    const i = s.indexOf(sub);
    return i >= 0
        ? { $tag: "Just", _0: i, $type: "Maybe" }
        : { $tag: "Nothing", $type: "Maybe" };
}

export function replace(from, to, s) {
    return s.split(from).join(to);
}

export function take(n, s) {
    return [...s].slice(0, n).join("");
}

export function drop(n, s) {
    return [...s].slice(n).join("");
}

export function repeat(n, s) {
    return s.repeat(n);
}

export function padLeft(n, ch, s) {
    return s.padStart(n, ch);
}

export function padRight(n, ch, s) {
    return s.padEnd(n, ch);
}

export function words(s) {
    const ws = s.trim().split(/\s+/).filter(x => x);
    return ws.reduceRight((t, h) => ({ $tag: "Cons", head: h, tail: t }), null);
}

export function lines(s) {
    const ls = s.split(/\r?\n/);
    return ls.reduceRight((t, h) => ({ $tag: "Cons", head: h, tail: t }), null);
}

export function charCode(s) {
    if (s.length === 0) return { $tag: "Nothing", $type: "Maybe" };
    return { $tag: "Just", _0: s.codePointAt(0), $type: "Maybe" };
}

export function fromCharCode(n) {
    if (n < 0 || n > 0x10FFFF) return { $tag: "Nothing", $type: "Maybe" };
    return { $tag: "Just", _0: String.fromCodePoint(n), $type: "Maybe" };
}

export function parseInt(s) {
    const n = globalThis.parseInt(s, 10);
    return isNaN(n)
        ? { $tag: "Nothing", $type: "Maybe" }
        : { $tag: "Just", _0: n, $type: "Maybe" };
}

export function parseFloat(s) {
    const n = globalThis.parseFloat(s);
    return isNaN(n)
        ? { $tag: "Nothing", $type: "Maybe" }
        : { $tag: "Just", _0: n, $type: "Maybe" };
}

export function reverse(s) {
    return [...s].reverse().join("");
}

export function toList(s) {
    return [...s].reduceRight((t, h) => ({ $tag: "Cons", head: h, tail: t }), null);
}

export function fromList(lst) {
    const chars = [];
    let cur = lst;
    while (cur !== null && cur.$tag === "Cons") { chars.push(cur.head); cur = cur.tail; }
    return chars.join("");
}
