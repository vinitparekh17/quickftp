export interface SelectableRow {
  path: string;
  isDir: boolean;
}
export interface SelectionResult {
  selected: string[];
  anchor: number;
}

export function pointerSelection(
  rows: SelectableRow[],
  selected: string[],
  anchor: number,
  index: number,
  modifiers: { ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }
): SelectionResult {
  const row = rows[index];
  if (!row || row.isDir) return { selected, anchor };
  if (modifiers.shiftKey && anchor >= 0) {
    const a = Math.min(anchor, index),
      b = Math.max(anchor, index);
    const range = rows
      .slice(a, b + 1)
      .filter((x) => !x.isDir)
      .map((x) => x.path);
    return {
      selected:
        modifiers.ctrlKey || modifiers.metaKey
          ? [...new Set([...selected, ...range])]
          : range,
      anchor,
    };
  }
  if (modifiers.ctrlKey || modifiers.metaKey)
    return {
      selected: selected.includes(row.path)
        ? selected.filter((p) => p !== row.path)
        : [...selected, row.path],
      anchor: index,
    };
  return { selected: [row.path], anchor: index };
}

export function keyboardSelection(
  rows: SelectableRow[],
  selected: string[],
  anchor: number,
  index: number,
  shiftKey: boolean
): SelectionResult {
  const row = rows[index];
  if (!row || row.isDir) return { selected, anchor };
  if (shiftKey && anchor >= 0) {
    const a = Math.min(anchor, index),
      b = Math.max(anchor, index);
    return {
      selected: rows
        .slice(a, b + 1)
        .filter((x) => !x.isDir)
        .map((x) => x.path),
      anchor,
    };
  }
  return { selected: [row.path], anchor: index };
}
