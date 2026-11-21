"use strict";

const assert = require("assert");
const { normalizeUserPath, pathCompareKey } = require("./dist/src/pathnorm.js");

assert.strictEqual("：".codePointAt(0), 0xff1a);
assert.strictEqual("＼".codePointAt(0), 0xff3c);

const screenshotDir = "E:\\qq音乐下载\\输出";

const accepted = [
  ["半角冒号", screenshotDir, screenshotDir],
  ["全角冒号", "E：\\qq音乐下载\\输出", screenshotDir],
  ["ASCII双引号", '"E:\\qq音乐下载\\输出"', screenshotDir],
  ["ASCII单引号", "'E:\\qq音乐下载\\输出'", screenshotDir],
  ["引号加全角冒号", '"E：\\qq音乐下载\\输出"', screenshotDir],
  ["首尾空格", `  ${screenshotDir}  `, screenshotDir],
  ["首尾制表符和换行", `\t${screenshotDir}\r\n`, screenshotDir],
  ["全角空格", `\u3000${screenshotDir}\u3000`, screenshotDir],
  ["不换行空格", `\u00a0${screenshotDir}\u00a0`, screenshotDir],
  ["引号内空白和全角冒号", '  "  E：\\qq音乐下载\\输出  "  ', screenshotDir],
  ["中文双引号", "“E:\\qq音乐下载\\输出”", screenshotDir],
  ["中文单引号", "‘E:\\qq音乐下载\\输出’", screenshotDir],
  ["直角引号加全角冒号", "「E：\\qq音乐下载\\输出」", screenshotDir],
  ["双直角引号", "『E:\\qq音乐下载\\输出』", screenshotDir],
  ["全角引号", "＂E:\\qq音乐下载\\输出＂", screenshotDir],
  ["全角单引号", "＇E:\\qq音乐下载\\输出＇", screenshotDir],
  ["书名号式引号", "«E:\\qq音乐下载\\输出»", screenshotDir],
  ["德文引号", "„E:\\qq音乐下载\\输出”", screenshotDir],
  ["嵌套引号", '"“E:\\qq音乐下载\\输出”"', screenshotDir],
  ["全角反斜杠", "E:\\qq音乐下载＼输出", screenshotDir],
  ["全角盘符全角冒号和全角反斜杠", "Ｅ：＼qq音乐下载＼输出", screenshotDir],
  ["小写全角盘符", "ｅ：\\qq音乐下载\\输出", "e:\\qq音乐下载\\输出"],
  ["正斜杠分隔", "E:/qq音乐下载/输出", "E:/qq音乐下载/输出"],
  ["带引号的UNC", '"\\\\server\\share\\qq音乐下载"', "\\\\server\\share\\qq音乐下载"],
  ["全角反斜杠UNC", "＼＼server＼share＼music", "\\\\server\\share\\music"],
  ["设备路径", "\\\\?\\E:\\qq音乐下载\\输出", "\\\\?\\E:\\qq音乐下载\\输出"],
];

const preserved = [
  "E:\\音乐\\歌曲：精选",
  "E:\\音乐\\2024／演唱会",
  "D:\\qq音乐下载\\输出\\歌曲：精选／现场",
];

const cleanedButNotLocal = [
  ["空字符串", "", ""],
  ["空白", " \t\u3000 ", ""],
  ["空引号", '""', ""],
  ["空中文引号", "“”", ""],
  ["相对路径", "qq音乐下载\\输出", "qq音乐下载\\输出"],
  ["引号包裹的相对路径", '  "qq音乐下载\\输出"  ', "qq音乐下载\\输出"],
  ["Unix绝对路径", "/home/user/输出", "/home/user/输出"],
  ["URL", "https://example.com/输出", "https://example.com/输出"],
  ["目录名中的全角冒号", "歌曲：精选", "歌曲：精选"],
  ["缺少盘符的全角冒号", "：\\qq音乐下载\\输出", "：\\qq音乐下载\\输出"],
  ["盘符后没有冒号", "E\\qq音乐下载\\输出", "E\\qq音乐下载\\输出"],
  ["file URL", "file:///E:/输出", "file:///E:/输出"],
  ["只有半边引号", '"E:\\qq音乐下载\\输出', '"E:\\qq音乐下载\\输出'],
];

let failed = 0;

function check(name, actual, expected) {
  try {
    assert.strictEqual(actual, expected);
  } catch (error) {
    failed += 1;
    console.error(`FAIL ${name}\n  实际: ${JSON.stringify(actual)}\n  期望: ${JSON.stringify(expected)}`);
  }
}

for (const [name, input, want] of accepted) {
  const got = normalizeUserPath(input);
  check(`接受/${name}`, got, want);
  check(`接受/${name}/稳定`, normalizeUserPath(got), got);
  check(`比较/${name}`, pathCompareKey(input), pathCompareKey(want));
}

for (const input of preserved) {
  check(`保留/${input}`, normalizeUserPath(input), input);
}

for (const [name, input, want] of cleanedButNotLocal) {
  check(`整理/${name}`, normalizeUserPath(input), want);
}

check(
  "全角冒号与半角冒号视为同一目录",
  pathCompareKey("E：\\qq音乐下载\\输出"),
  pathCompareKey(screenshotDir),
);
check(
  "引号与空白不影响目录比较",
  pathCompareKey('  "E:\\qq音乐下载\\输出\\"  '),
  pathCompareKey(screenshotDir),
);
check(
  "不同盘符仍然区分",
  pathCompareKey("D:\\qq音乐下载\\输出") === pathCompareKey(screenshotDir),
  false,
);

if (failed > 0) {
  console.error(`${failed} 项前端路径规范化断言失败`);
  process.exit(1);
}

console.log(`前端路径规范化通过：${accepted.length} 组与后端相同的结果，${preserved.length} 组目录名保留，${cleanedButNotLocal.length} 组整理后仍不是盘符（拒绝与否由 Go 测试断言）`);
