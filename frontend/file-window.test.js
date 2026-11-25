"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { fileWindow, FILE_ROW_HEIGHT } = require("./dist/src/file-window.js");

function indexesIn(window) {
  const found = [];
  for (let i = window.start; i < window.end; i++) found.push(i);
  return found;
}

test("426 个文件可以滚动到最后一项，不会停在 400", () => {
  const count = 426;
  const viewport = 290;
  const bottom = Math.max(0, count * FILE_ROW_HEIGHT - viewport);
  const atTop = fileWindow(0, viewport, count, FILE_ROW_HEIGHT, 8);
  const atOldCap = fileWindow(400 * FILE_ROW_HEIGHT, viewport, count, FILE_ROW_HEIGHT, 8);
  const atBottom = fileWindow(bottom, viewport, count, FILE_ROW_HEIGHT, 8);

  assert.equal(atTop.start, 0);
  assert.ok(atTop.end < count, "首屏只绘制窗口内的行");
  assert.ok(indexesIn(atOldCap).includes(400), "第 401 个文件必须能进入窗口");
  assert.equal(atBottom.end, count);
  assert.ok(atBottom.start <= 425);
  assert.ok(indexesIn(atBottom).includes(425));
  assert.ok(atBottom.end > 400);
});

test("从顶滚到底，每个序号都会出现在某个窗口里", () => {
  for (const count of [1, 40, 400, 401, 426, 2500]) {
    const viewport = 290;
    const seen = new Set();
    const maxScroll = Math.max(0, count * FILE_ROW_HEIGHT - viewport);
    for (let top = 0; top <= maxScroll; top += FILE_ROW_HEIGHT) {
      const window = fileWindow(top, viewport, count, FILE_ROW_HEIGHT, 4);
      assert.ok(window.start >= 0 && window.end <= count && window.start < window.end);
      assert.equal(window.totalHeight, count * FILE_ROW_HEIGHT);
      for (let i = window.start; i < window.end; i++) seen.add(i);
    }
    const bottom = fileWindow(maxScroll, viewport, count, FILE_ROW_HEIGHT, 4);
    assert.equal(bottom.end, count, `count=${count} 滚到底应包含最后一项`);
    assert.equal(seen.size, count, `count=${count} 有序号没有进入任何窗口`);
  }
});

test("空列表和超出底部的滚动位置也不会造出虚假行", () => {
  assert.deepEqual(fileWindow(0, 290, 0, FILE_ROW_HEIGHT, 8), { start: 0, end: 0, offset: 0, totalHeight: 0 });
  const over = fileWindow(10_000_000, 290, 426, FILE_ROW_HEIGHT, 8);
  assert.equal(over.end, 426);
  assert.ok(over.start < 426);
  assert.equal(over.offset, over.start * FILE_ROW_HEIGHT);
});
