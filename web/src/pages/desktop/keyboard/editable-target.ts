// A field of the page itself owns the keyboard while it has focus. A key typed
// into it is the user's text for the page, so it is neither forwarded to the
// host nor kept from the field (ironkvm-dist#187). The dialogs that hold a
// keyboard lock already stop the forwarding as a whole; this covers every
// field outside one, and both rules can hold at once without disagreeing.

// The part of an element this decision reads. It is spelled out so the test
// can pass plain objects, as the node test runner has no DOM.
export interface EditableCandidate {
  readonly tagName?: unknown;
  readonly type?: unknown;
  readonly isContentEditable?: unknown;
  getClientRects?: () => { readonly length: number };
}

// Input types that take no typing. Focus stays on a checkbox or a button after
// a click, and treating those as fields would leave the host without a
// keyboard until the user clicked somewhere else.
const NON_TEXT_INPUT_TYPES = new Set([
  'button',
  'checkbox',
  'color',
  'file',
  'image',
  'radio',
  'range',
  'reset',
  'submit'
]);

/**
 * Whether a key event's target is a field of the page that takes the key
 * itself: a text input, a textarea, a select or a contenteditable element.
 *
 * A field that is no longer rendered does not count. Not every browser moves
 * the focus away when a popover hides its field, and the keys would otherwise
 * go to a field nobody can see.
 */
export function isEditableTarget(target: unknown): boolean {
  if (typeof target !== 'object' || target === null) return false;
  const element = target as EditableCandidate;

  if (!takesKeys(element)) return false;

  if (typeof element.getClientRects === 'function') {
    return element.getClientRects().length > 0;
  }
  return true;
}

function takesKeys(element: EditableCandidate): boolean {
  if (element.isContentEditable === true) return true;
  if (typeof element.tagName !== 'string') return false;

  switch (element.tagName.toUpperCase()) {
    case 'TEXTAREA':
    case 'SELECT':
      return true;
    case 'INPUT': {
      const type = typeof element.type === 'string' ? element.type.toLowerCase() : 'text';
      return !NON_TEXT_INPUT_TYPES.has(type);
    }
    default:
      return false;
  }
}
