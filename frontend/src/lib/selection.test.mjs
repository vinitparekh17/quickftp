import assert from 'node:assert/strict';
import { pointerSelection, keyboardSelection } from './selection.ts';

const rows = [
  { path: '/root/a', isDir: false },
  { path: '/root/folder', isDir: true },
  { path: '/root/b', isDir: false },
  { path: '/root/c', isDir: false },
];
{
  assert.deepEqual(
    pointerSelection(rows, ['/root/b'], 2, 2, {
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
    }),
    { selected: ['/root/b'], anchor: 2 }
  );
  assert.deepEqual(
    pointerSelection(rows, ['/root/a'], 0, 2, {
      ctrlKey: true,
      metaKey: false,
      shiftKey: false,
    }),
    { selected: ['/root/a', '/root/b'], anchor: 2 }
  );
  assert.deepEqual(
    pointerSelection(rows, ['/root/a', '/root/b'], 2, 2, {
      ctrlKey: true,
      metaKey: false,
      shiftKey: false,
    }),
    { selected: ['/root/a'], anchor: 2 }
  );
}
{
  assert.deepEqual(
    pointerSelection(rows, ['/root/a'], 0, 3, {
      ctrlKey: false,
      metaKey: false,
      shiftKey: true,
    }),
    { selected: ['/root/a', '/root/folder', '/root/b', '/root/c'], anchor: 0 }
  );
  assert.deepEqual(keyboardSelection(rows, ['/root/a'], 0, 3, true), {
    selected: ['/root/a', '/root/folder', '/root/b', '/root/c'],
    anchor: 0,
  });
}
{
  assert.deepEqual(
    pointerSelection(rows, ['/root/c'], 0, 3, {
      ctrlKey: true,
      metaKey: false,
      shiftKey: true,
    }),
    { selected: ['/root/c', '/root/a', '/root/folder', '/root/b'], anchor: 0 }
  );
  assert.deepEqual(
    pointerSelection(rows, ['/root/a'], 0, 1, {
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
    }),
    { selected: ['/root/folder'], anchor: 1 }
  );
}
console.log(
  'Selection tests passed: single, Ctrl, Shift, Shift+Ctrl, keyboard range, folder inclusion.'
);
