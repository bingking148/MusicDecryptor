// 用户输入的 Windows 路径规范化。
// 规则需与 engine/pathnorm.go 的 NormalizeUserPath 保持一致。
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  } else {
    root.normalizeUserPath = api.normalizeUserPath;
    root.pathCompareKey = api.pathCompareKey;
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  const QUOTE_PAIRS = [
    ['"', '"'],
    ["'", "'"],
    ["“", "”"],
    ["‘", "’"],
    ["„", "“"],
    ["„", "”"],
    ["«", "»"],
    ["「", "」"],
    ["『", "』"],
    ["＂", "＂"],
    ["＇", "＇"],
  ];

  function normalizeUserPath(path) {
    let value = stripWrappingQuotes(String(path ?? ""));
    value = value.replaceAll("＼", "\\");
    const chars = Array.from(value);
    if (chars.length >= 2) {
      const letter = foldFullwidthLetter(chars[0]);
      if (letter && (chars[1] === ":" || chars[1] === "：")) {
        chars[0] = letter;
        chars[1] = ":";
        value = chars.join("");
      }
    }
    return value;
  }

  // 供「来源目录和输出目录是否相同」比较。大小写与末尾分隔符不影响判断。
  function pathCompareKey(path) {
    return normalizeUserPath(path).replace(/[\\/]+$/, "").replace(/\//g, "\\").toLocaleLowerCase();
  }

  function stripWrappingQuotes(path) {
    let value = path.trim();
    while (true) {
      const chars = Array.from(value);
      if (chars.length < 2) return value;
      const matched = QUOTE_PAIRS.some((pair) => chars[0] === pair[0] && chars[chars.length - 1] === pair[1]);
      if (!matched) return value;
      value = chars.slice(1, -1).join("").trim();
    }
  }

  function foldFullwidthLetter(ch) {
    const code = ch.codePointAt(0);
    if (code >= 0x41 && code <= 0x5a) return ch;
    if (code >= 0x61 && code <= 0x7a) return ch;
    if (code >= 0xff21 && code <= 0xff3a) return String.fromCodePoint(0x41 + (code - 0xff21));
    if (code >= 0xff41 && code <= 0xff5a) return String.fromCodePoint(0x61 + (code - 0xff41));
    return "";
  }

  return { normalizeUserPath, pathCompareKey };
});
