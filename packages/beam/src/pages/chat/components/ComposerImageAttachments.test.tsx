import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import type { PendingImageAttachment } from '../lib/imageAttachments';
import {
  AttachImageButton,
  ComposerImageAttachments,
  ImageAttachmentNoticeView,
} from './ComposerImageAttachments';

// Pin the language so labels are deterministic (the app default is German).
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * PermissionCard.test.tsx) — these render to static markup, which pins the
 * chip/button/notice DOM contract; the add/remove/paste STATE logic lives in
 * pure helpers (lib/imageAttachments.ts) and the reducer/controller seams,
 * each covered by their own suites.
 */
const images: PendingImageAttachment[] = [
  { id: 'img-1', name: 'shot.png', data: 'aGVsbG8=', mimeType: 'image/png' },
  { id: 'img-2', name: '', data: 'd29ybGQ=', mimeType: 'image/jpeg' },
];

describe('ComposerImageAttachments', () => {
  it('renders one removable thumbnail chip per pending attachment (data-URI img + labeled remove button)', () => {
    const html = renderToStaticMarkup(
      createElement(ComposerImageAttachments, { images, onRemove: vi.fn() }),
    );
    expect(html).toContain('data-testid="composer-image-attachments"');
    expect((html.match(/data-testid="composer-image-attachment"/g) ?? []).length).toBe(2);
    expect(html).toContain('src="data:image/png;base64,aGVsbG8="');
    expect(html).toContain('src="data:image/jpeg;base64,d29ybGQ="');
    // Remove button per chip, named after the file…
    expect(html).toContain('aria-label="Remove attachment: shot.png"');
    // …with the localized fallback for a nameless clipboard image.
    expect(html).toContain('aria-label="Remove attachment: Image"');
  });

  it('renders nothing when no attachments are pending (safe to pass unconditionally)', () => {
    const html = renderToStaticMarkup(
      createElement(ComposerImageAttachments, { images: [], onRemove: vi.fn() }),
    );
    expect(html).toBe('');
  });

  it('never removes merely by rendering', () => {
    const onRemove = vi.fn();
    renderToStaticMarkup(createElement(ComposerImageAttachments, { images, onRemove }));
    expect(onRemove).not.toHaveBeenCalled();
  });
});

describe('AttachImageButton', () => {
  it('renders the labeled attach button driving a hidden image-only file input', () => {
    const html = renderToStaticMarkup(createElement(AttachImageButton, { onFiles: vi.fn() }));
    expect(html).toContain('data-testid="composer-attach-image"');
    expect(html).toContain('aria-label="Attach image"');
    expect(html).toContain('data-testid="composer-image-input"');
    expect(html).toContain('type="file"');
    expect(html).toContain('accept="image/*"');
    expect(html).toContain('multiple');
  });

  it('is disableable alongside the composer', () => {
    const html = renderToStaticMarkup(
      createElement(AttachImageButton, { onFiles: vi.fn(), disabled: true }),
    );
    expect(html).toMatch(/<button[^>]*disabled/);
  });
});

describe('ImageAttachmentNoticeView', () => {
  it('maps each rejection to its localized notice', () => {
    expect(
      renderToStaticMarkup(createElement(ImageAttachmentNoticeView, { notice: 'not_supported' })),
    ).toContain('This agent does not accept images');
    expect(
      renderToStaticMarkup(createElement(ImageAttachmentNoticeView, { notice: 'too_large' })),
    ).toContain('too large');
    expect(
      renderToStaticMarkup(createElement(ImageAttachmentNoticeView, { notice: 'process_failed' })),
    ).toContain('could not be processed');
  });

  it('renders nothing without a notice', () => {
    expect(renderToStaticMarkup(createElement(ImageAttachmentNoticeView, { notice: null }))).toBe(
      '',
    );
  });
});
