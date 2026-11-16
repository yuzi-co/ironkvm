// A button released while the page is not looking is never reported: the
// mouseup goes to whatever window has the focus by then. So a button still
// down when the window loses focus or the tab is hidden is let go on the host
// at that moment, the way the keyboard already lets go of its keys
// (ironkvm-dist#187).

/**
 * Whether a page event means the browser may stop delivering button releases.
 * A visibilitychange counts only when it hides the page; the one that shows it
 * again has nothing left to release.
 */
export function leavesPage(eventType: string, visibilityState: string): boolean {
  switch (eventType) {
    case 'blur':
      return true;
    case 'visibilitychange':
      return visibilityState === 'hidden';
    default:
      return false;
  }
}

/**
 * Calls release when the window loses focus or the tab is hidden, and returns
 * the function that stops listening.
 */
export function onPageLeave(release: () => void): () => void {
  const handle = (event: Event) => {
    if (leavesPage(event.type, document.visibilityState)) {
      release();
    }
  };

  window.addEventListener('blur', handle);
  document.addEventListener('visibilitychange', handle);

  return () => {
    window.removeEventListener('blur', handle);
    document.removeEventListener('visibilitychange', handle);
  };
}
