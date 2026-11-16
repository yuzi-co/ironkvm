import assert from 'node:assert/strict';
import { test } from 'node:test';

import { leavesPage } from './page-leave.ts';

test('a window that loses focus releases the buttons', () => {
  assert.equal(leavesPage('blur', 'visible'), true);
  assert.equal(leavesPage('blur', 'hidden'), true);
});

test('a tab that is hidden releases the buttons', () => {
  assert.equal(leavesPage('visibilitychange', 'hidden'), true);
});

test('a tab that is shown again releases nothing', () => {
  assert.equal(leavesPage('visibilitychange', 'visible'), false);
});

test('other events release nothing', () => {
  assert.equal(leavesPage('focus', 'visible'), false);
  assert.equal(leavesPage('mouseleave', 'hidden'), false);
});
