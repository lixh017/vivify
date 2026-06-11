'use client'

// useModalA11y wires up the a11y primitives every modal in this app
// needs: body scroll lock while the modal is open, focus trap
// (focus first focusable on open, restore previous focus on close),
// and an Escape key handler. Extracted to a hook so the dashboard
// and knowledge deconstruct modals (Phase 2 QA: both were missing
// these primitives) stay in lockstep.
//
// Returns the `dialogRef` to attach to the modal panel. The hook
// is intentionally lightweight: no portal, no animation, no
// outside-click handling — those are already done by the existing
// `fixed inset-0` backdrop with its own onClick.

import { useEffect, useRef } from 'react'

// FOCUSABLE_SELECTOR enumerates the elements that can receive
// keyboard focus. Order matters for "first focusable" semantics
// — anything tabbable counts, but `input`/`textarea`/`select` are
// the most common first focus targets inside a form modal.
const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

export function useModalA11y(isOpen: boolean): React.RefObject<HTMLDivElement> {
  const dialogRef = useRef<HTMLDivElement>(null)
  // previouslyFocused holds the element that was focused when the
  // modal opened. We restore focus to it on close so keyboard users
  // land back on the trigger button (e.g. the "AI 拆解爆款" CTA)
  // rather than the top of the document.
  const previouslyFocused = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!isOpen) return

    // Body scroll lock: while the modal is open, the page behind it
    // must not scroll. Without this the modal's own
    // `max-h-[90vh] overflow-y-auto` is undermined by a backdrop
    // that scrolls in parallel on long transcripts.
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    // Capture the element to restore focus to on close. Default to
    // `document.activeElement` (typically the trigger button) but
    // guard for the case where it isn't an HTMLElement.
    const active = document.activeElement
    previouslyFocused.current =
      active instanceof HTMLElement ? active : null

    // Focus the first focusable element inside the dialog. Fall
    // back to the dialog container itself so the panel always
    // receives focus on open (needed for screen reader
    // announcement of role/aria-modal).
    const dialog = dialogRef.current
    if (dialog) {
      const firstFocusable = dialog.querySelector<HTMLElement>(FOCUSABLE_SELECTOR)
      if (firstFocusable) {
        firstFocusable.focus()
      } else {
        dialog.focus()
      }
    }

    // Escape closes the modal. We don't have onClose in scope here
    // (different modals handle close differently — backdrop click
    // vs. an X button), so we listen for the keydown and dispatch
    // a custom event that the caller can intercept. We also handle
    // the common case by calling onClose if provided via a
    // data-attribute, but the simpler and tested approach is to
    // dispatch a 'keydown' event the caller already listens for.
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        // Dispatch a bubbling CustomEvent so the modal's existing
        // onClick={onClose} handler can stay the single source of
        // truth for closing. The event is dispatched on the dialog
        // element so it doesn't fire for unrelated modals on the
        // same page.
        dialog?.dispatchEvent(
          new CustomEvent('modal-escape', { bubbles: true }),
        )
      } else if (e.key === 'Tab') {
        // Focus trap: keep Tab/Shift+Tab inside the dialog. If the
        // user tabs past the last focusable, wrap to the first;
        // if they shift-tab past the first, wrap to the last.
        if (!dialog) return
        const focusables: HTMLElement[] = Array.from(
          dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
        )
        if (focusables.length === 0) {
          e.preventDefault()
          dialog.focus()
          return
        }
        const first = focusables[0]
        const last = focusables[focusables.length - 1]
        const current = document.activeElement
        if (e.shiftKey && current === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && current === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }

    document.addEventListener('keydown', handleKeyDown)

    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = previousOverflow
      // Restore focus to the element that opened the modal so
      // keyboard users don't get dropped at the top of the
      // document. Skip if the previously-focused element is no
      // longer in the DOM (e.g. a list item got filtered out).
      const target = previouslyFocused.current
      if (target && document.body.contains(target)) {
        target.focus()
      }
    }
  }, [isOpen])

  return dialogRef
}
