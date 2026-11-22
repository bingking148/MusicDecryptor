// 文件列表的滚动窗口。界面只绘制视口附近的行，但滚动范围覆盖全部文件，没有条数上限。
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.FileListWindow = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  const FILE_ROW_HEIGHT = 43;
  const FILE_ROW_OVERSCAN = 8;

  // fileWindow 返回 [start, end) 以及整表高度。end 可以等于 count，不会停在某个固定条数。
  function fileWindow(scrollTop, viewportHeight, count, rowHeight, overscan) {
    const total = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
    const row = Number.isFinite(rowHeight) && rowHeight > 0 ? rowHeight : 1;
    const extra = Number.isFinite(overscan) ? Math.max(0, Math.floor(overscan)) : 0;
    const viewport = Number.isFinite(viewportHeight) && viewportHeight > 0 ? viewportHeight : row;
    if (total === 0) return { start: 0, end: 0, offset: 0, totalHeight: 0 };

    const totalHeight = total * row;
    const maxScroll = Math.max(0, totalHeight - viewport);
    const top = Math.min(Math.max(0, Number(scrollTop) || 0), maxScroll);
    const first = Math.floor(top / row);
    const visibleCount = Math.max(1, Math.ceil(viewport / row));
    let start = first - extra;
    if (start < 0) start = 0;
    let end = first + visibleCount + extra;
    if (end > total) end = total;
    if (end <= start) end = Math.min(total, start + 1);
    return { start, end, offset: start * row, totalHeight };
  }

  return { FILE_ROW_HEIGHT, FILE_ROW_OVERSCAN, fileWindow };
});
