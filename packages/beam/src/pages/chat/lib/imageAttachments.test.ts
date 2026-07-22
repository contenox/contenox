import { describe, expect, it } from 'vitest';
import {
  base64ByteLength,
  canAttachImages,
  dataTransferHasImages,
  imageFilesFromDataTransfer,
  imagePartFromDataUrl,
  isAttachableImageType,
  MAX_IMAGE_BYTES,
  type DataTransferLike,
} from './imageAttachments';

/** Structural stand-in for a File — the extraction helpers only ever read `.type`. */
function fakeFile(type: string, name = 'f'): File {
  return { type, name } as File;
}

/** A paste-style payload: `items` entries whose `getAsFile()` materializes the file. */
function pastePayload(...types: string[]): DataTransferLike {
  return {
    items: types.map(type => ({
      kind: type === '' ? 'string' : 'file',
      type,
      getAsFile: () => (type === '' ? null : fakeFile(type)),
    })),
  };
}

describe('canAttachImages — PromptCapabilities gating', () => {
  it('is true ONLY for an explicit image:true advertisement', () => {
    expect(canAttachImages({ image: true })).toBe(true);
  });
  it('is false for image:false, an empty capability set, and a missing one', () => {
    expect(canAttachImages({ image: false })).toBe(false);
    expect(canAttachImages({})).toBe(false);
    expect(canAttachImages(undefined)).toBe(false);
  });
});

describe('isAttachableImageType', () => {
  it('accepts the browser-decodable raster formats', () => {
    for (const t of ['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/bmp']) {
      expect(isAttachableImageType(t)).toBe(true);
    }
  });
  it('rejects SVG (vector, flaky to rasterize) and non-images', () => {
    expect(isAttachableImageType('image/svg+xml')).toBe(false);
    expect(isAttachableImageType('text/plain')).toBe(false);
    expect(isAttachableImageType('application/pdf')).toBe(false);
  });
});

describe('imageFilesFromDataTransfer', () => {
  it('extracts image files from a paste payload (items), skipping the text component of a mixed paste', () => {
    const files = imageFilesFromDataTransfer(pastePayload('', 'image/png'));
    expect(files.map(f => f.type)).toEqual(['image/png']);
  });

  it('extracts multiple images in payload order', () => {
    const files = imageFilesFromDataTransfer(pastePayload('image/png', 'image/jpeg'));
    expect(files.map(f => f.type)).toEqual(['image/png', 'image/jpeg']);
  });

  it('falls back to `files` for drop payloads, filtering non-images', () => {
    const dt: DataTransferLike = {
      files: [fakeFile('image/webp'), fakeFile('text/plain'), fakeFile('image/png')],
    };
    expect(imageFilesFromDataTransfer(dt).map(f => f.type)).toEqual(['image/webp', 'image/png']);
  });

  it('returns [] for a plain-text payload (the composer lets the default paste run)', () => {
    expect(imageFilesFromDataTransfer(pastePayload(''))).toEqual([]);
    expect(imageFilesFromDataTransfer({ files: [fakeFile('text/plain')] })).toEqual([]);
    expect(imageFilesFromDataTransfer(null)).toEqual([]);
  });
});

describe('dataTransferHasImages — dragover acceptance (files not yet materialized)', () => {
  it('detects an image drag by item TYPE alone', () => {
    // During dragover getAsFile() returns null — the type is all we have.
    const dt = { items: [{ kind: 'file', type: 'image/png' }] };
    expect(dataTransferHasImages(dt)).toBe(true);
  });
  it('is false for text drags, non-image files, and empty payloads', () => {
    expect(dataTransferHasImages({ items: [{ kind: 'string', type: 'text/plain' }] })).toBe(false);
    expect(dataTransferHasImages({ items: [{ kind: 'file', type: 'application/pdf' }] })).toBe(
      false,
    );
    expect(dataTransferHasImages(null)).toBe(false);
  });
});

describe('base64ByteLength', () => {
  it('computes decoded sizes including padding variants', () => {
    expect(base64ByteLength('')).toBe(0);
    expect(base64ByteLength('aGVsbG8=')).toBe(5); // "hello"
    expect(base64ByteLength('aGVsbG8h')).toBe(6); // "hello!"
    expect(base64ByteLength('aGVsbG8hIQ==')).toBe(7); // "hello!!"
  });
  it('agrees with the byte cap constant scale (a 5 MB payload is ~6.67 MB of base64)', () => {
    const b64Len = Math.ceil(MAX_IMAGE_BYTES / 3) * 4;
    expect(base64ByteLength('A'.repeat(b64Len))).toBeGreaterThanOrEqual(MAX_IMAGE_BYTES);
  });
});

describe('imagePartFromDataUrl', () => {
  it('parses a data: URL into the ACP wire form (raw base64, no prefix)', () => {
    expect(imagePartFromDataUrl('data:image/png;base64,aGVsbG8=')).toEqual({
      mimeType: 'image/png',
      data: 'aGVsbG8=',
    });
  });
  it('returns null for non-base64 or malformed URLs', () => {
    expect(imagePartFromDataUrl('data:text/plain,hello')).toBeNull();
    expect(imagePartFromDataUrl('https://example.test/x.png')).toBeNull();
    expect(imagePartFromDataUrl('')).toBeNull();
  });
});
