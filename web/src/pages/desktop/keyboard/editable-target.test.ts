import assert from 'node:assert/strict';
import { test } from 'node:test';

import { isEditableTarget } from './editable-target.ts';

function element(tagName: string, extra: Record<string, unknown> = {}) {
  return { tagName, isContentEditable: false, ...extra };
}

test('a text input, a textarea and a select take their own keys', () => {
  assert.equal(isEditableTarget(element('INPUT', { type: 'text' })), true);
  assert.equal(isEditableTarget(element('TEXTAREA', { type: 'textarea' })), true);
  assert.equal(isEditableTarget(element('SELECT', { type: 'select-one' })), true);
});

test('every typing kind of input counts, whatever its type', () => {
  for (const type of ['password', 'search', 'number', 'url', 'email', 'tel', 'date', 'time']) {
    assert.equal(isEditableTarget(element('INPUT', { type })), true, type);
  }
  // An input with an unknown type, or none, is a text input to the browser.
  assert.equal(isEditableTarget(element('INPUT', { type: 'no-such-type' })), true);
  assert.equal(isEditableTarget(element('INPUT')), true);
});

test('an input that takes no typing leaves the keys with the host', () => {
  const types = [
    'button',
    'checkbox',
    'color',
    'file',
    'image',
    'radio',
    'range',
    'reset',
    'submit'
  ];
  for (const type of types) {
    assert.equal(isEditableTarget(element('INPUT', { type })), false, type);
  }
});

test('a contenteditable element takes its own keys', () => {
  assert.equal(isEditableTarget(element('DIV', { isContentEditable: true })), true);
  assert.equal(isEditableTarget(element('SPAN', { isContentEditable: true })), true);
});

test('the rest of the page leaves the keys with the host', () => {
  assert.equal(isEditableTarget(element('BODY')), false);
  assert.equal(isEditableTarget(element('VIDEO')), false);
  assert.equal(isEditableTarget(element('BUTTON', { type: 'button' })), false);
  assert.equal(isEditableTarget(element('DIV')), false);
  assert.equal(isEditableTarget(element('A')), false);
});

test('a target that is not an element is not a field', () => {
  assert.equal(isEditableTarget(null), false);
  assert.equal(isEditableTarget(undefined), false);
  assert.equal(isEditableTarget('INPUT'), false);
  // The document and the window can be a key event's target too.
  assert.equal(isEditableTarget({}), false);
});

test('the tag name is read in either case', () => {
  assert.equal(isEditableTarget(element('input', { type: 'TEXT' })), true);
  assert.equal(isEditableTarget(element('textarea')), true);
  assert.equal(isEditableTarget(element('input', { type: 'CHECKBOX' })), false);
});

test('a field that is not rendered does not hold the keyboard', () => {
  const hidden = element('INPUT', { type: 'text', getClientRects: () => ({ length: 0 }) });
  const shown = element('INPUT', { type: 'text', getClientRects: () => ({ length: 1 }) });
  assert.equal(isEditableTarget(hidden), false);
  assert.equal(isEditableTarget(shown), true);
});
