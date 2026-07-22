/**
 * The composer's image-attachment pipeline: capability gating, extracting
 * image files from paste/drop payloads, and downscaling/re-encoding them into
 * the ACP `ContentBlock` image wire form (raw base64 + mime type, no `data:`
 * prefix — see libacp's `NewImageContent`). The pure parts (gating, payload
 * extraction, data-URL parsing, byte accounting) live at the top and are what
 * the tests exercise; only `prepareImageAttachment` at the bottom touches
 * browser APIs (bitmap decode + canvas re-encode). No React, no DOM state —
 * the component (ChatSessionTab / ComposerImageAttachments.tsx) wires this to
 * its own `useState`, exactly like mentions.ts and its menu.
 */
import type { ImagePart, PromptCapabilities } from '../../../lib/acp';

/** Long-edge cap (px): larger images are downscaled before encoding — matches what vision models resolve anyway, and keeps the base64-in-JSON frames sane. */
export const MAX_IMAGE_EDGE_PX = 1568;

/** Hard post-encode cap on the image payload (bytes, decoded size). An image still exceeding this after downscale + JPEG re-encode is rejected. */
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;

/**
 * Raster formats the pipeline accepts as INPUT. Everything is re-encoded to
 * PNG/JPEG when it needs downscaling, so this is about what the browser can
 * reliably decode — SVG is deliberately excluded (a vector document, not a
 * screenshot; canvas-rasterizing it from a File is flaky across browsers).
 */
const ATTACHABLE_IMAGE_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/webp',
  'image/gif',
  'image/bmp',
]);

/** Whether the session's agent advertises image prompt support (`initialize` → `agentCapabilities.promptCapabilities.image`). Absent/false = no attach UI, paste/drop rejected. */
export function canAttachImages(capabilities: PromptCapabilities | undefined): boolean {
  return capabilities?.image === true;
}

export function isAttachableImageType(mimeType: string): boolean {
  return ATTACHABLE_IMAGE_TYPES.has(mimeType);
}

/**
 * One pending composer attachment: the wire payload (`ImagePart`) plus
 * client-local presentation fields. `id` never leaves the browser — it keys
 * the thumbnail chips and removal; `name` is the original filename (may be
 * empty for clipboard images — the UI falls back to a localized label).
 */
export interface PendingImageAttachment extends ImagePart {
  id: string;
  name: string;
}

/** Why an image could not become an attachment — the UI maps each reason to a localized notice. */
export type ImageAttachmentErrorReason = 'unsupported' | 'too_large' | 'process_failed';

export class ImageAttachmentError extends Error {
  constructor(readonly reason: ImageAttachmentErrorReason) {
    super(`image attachment rejected: ${reason}`);
    this.name = 'ImageAttachmentError';
  }
}

/**
 * Minimal structural view of `DataTransfer` (paste's `clipboardData`, drop's
 * `dataTransfer`) so extraction is testable without a DOM. Paste payloads
 * carry `items` (kind `'file'`); drop payloads reliably carry `files`.
 */
export interface DataTransferLike {
  items?: ArrayLike<{ kind: string; type: string; getAsFile(): File | null }>;
  files?: ArrayLike<File>;
}

/**
 * The attachable image files in a paste/drop payload, in payload order.
 * Non-image entries (the text component of a mixed paste, dropped folders…)
 * are simply skipped — they are not an error, the caller handles them (or
 * lets the default paste behavior run) as usual.
 */
export function imageFilesFromDataTransfer(dt: DataTransferLike | null | undefined): File[] {
  if (!dt) return [];
  const out: File[] = [];
  if (dt.items && dt.items.length > 0) {
    for (let i = 0; i < dt.items.length; i++) {
      const item = dt.items[i];
      if (item.kind !== 'file' || !isAttachableImageType(item.type)) continue;
      const file = item.getAsFile();
      if (file) out.push(file);
    }
    if (out.length > 0) return out;
  }
  if (dt.files) {
    for (let i = 0; i < dt.files.length; i++) {
      const file = dt.files[i];
      if (isAttachableImageType(file.type)) out.push(file);
    }
  }
  return out;
}

/**
 * Whether a drag payload contains at least one attachable image, judged by the
 * item TYPES only — usable during `dragover`, where `getAsFile()` still
 * returns null (files materialize only on drop). Gates `preventDefault` so the
 * composer becomes a drop target exactly for image drags.
 */
export function dataTransferHasImages(
  dt: { items?: ArrayLike<{ kind: string; type: string }> } | null | undefined,
): boolean {
  if (!dt?.items) return false;
  for (let i = 0; i < dt.items.length; i++) {
    const item = dt.items[i];
    if (item.kind === 'file' && isAttachableImageType(item.type)) return true;
  }
  return false;
}

/** Decoded byte length of a base64 payload (without materializing it). */
export function base64ByteLength(b64: string): number {
  if (b64 === '') return 0;
  let padding = 0;
  if (b64.endsWith('==')) padding = 2;
  else if (b64.endsWith('=')) padding = 1;
  return (b64.length * 3) / 4 - padding;
}

/** Parses a `data:<mime>;base64,<data>` URL into the ACP wire form (raw base64, no prefix), or null for anything else. */
export function imagePartFromDataUrl(dataUrl: string): ImagePart | null {
  const m = /^data:([^;,]+);base64,(.+)$/.exec(dataUrl);
  if (!m) return null;
  return { mimeType: m[1], data: m[2] };
}

let attachmentCounter = 0;
/** Monotonic client-local id — unique per browser tab, which is all a chip key needs to be (mirrors acpWorkspaceController's `nextId`). */
function nextAttachmentId(): string {
  attachmentCounter += 1;
  return `img-${attachmentCounter}`;
}

/** Reads a File into a `data:` URL (browser-only). */
function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error('file read failed'));
    reader.onload = () => resolve(String(reader.result));
    reader.readAsDataURL(file);
  });
}

/**
 * Turns a user-supplied image file into a pending attachment (browser-only —
 * decodes via `createImageBitmap`, re-encodes via canvas):
 *
 *  - already small enough (long edge ≤ {@link MAX_IMAGE_EDGE_PX}, ≤
 *    {@link MAX_IMAGE_BYTES}, PNG/JPEG): passed through byte-identical.
 *  - otherwise: downscaled to the long-edge cap and re-encoded — PNG stays
 *    PNG (transparency), everything else becomes JPEG; if the result still
 *    busts the byte cap it is retried as harder-compressed JPEG, and rejected
 *    (`too_large`) only when even that doesn't fit.
 *
 * Throws {@link ImageAttachmentError} with a UI-mappable `reason`.
 */
export async function prepareImageAttachment(file: File): Promise<PendingImageAttachment> {
  if (!isAttachableImageType(file.type)) throw new ImageAttachmentError('unsupported');

  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    throw new ImageAttachmentError('process_failed');
  }

  try {
    const scale = Math.min(1, MAX_IMAGE_EDGE_PX / Math.max(bitmap.width, bitmap.height));
    const isPassthroughType = file.type === 'image/png' || file.type === 'image/jpeg';
    if (scale === 1 && isPassthroughType && file.size <= MAX_IMAGE_BYTES) {
      const part = imagePartFromDataUrl(await readFileAsDataUrl(file));
      if (!part) throw new ImageAttachmentError('process_failed');
      return { id: nextAttachmentId(), name: file.name, ...part };
    }

    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new ImageAttachmentError('process_failed');
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);

    // PNG keeps transparency; everything else re-encodes as JPEG. A PNG that
    // busts the byte cap falls through to JPEG passes (flattened) rather than
    // being rejected outright.
    const attempts: Array<() => string> =
      file.type === 'image/png'
        ? [
            () => canvas.toDataURL('image/png'),
            () => canvas.toDataURL('image/jpeg', 0.9),
            () => canvas.toDataURL('image/jpeg', 0.8),
          ]
        : [() => canvas.toDataURL('image/jpeg', 0.9), () => canvas.toDataURL('image/jpeg', 0.8)];
    for (const attempt of attempts) {
      const part = imagePartFromDataUrl(attempt());
      if (part && base64ByteLength(part.data) <= MAX_IMAGE_BYTES) {
        return { id: nextAttachmentId(), name: file.name, ...part };
      }
    }
    throw new ImageAttachmentError('too_large');
  } finally {
    bitmap.close();
  }
}
