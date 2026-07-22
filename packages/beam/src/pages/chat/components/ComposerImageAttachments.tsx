/**
 * The composer's image-attachment surface: `useImageAttachments` owns the
 * pending-attachment state + paste/drop/file-pick handlers (capability-gated —
 * see `canAttachImages`), `ComposerImageAttachments` renders the removable
 * thumbnail chips above the textarea (via `ChatComposer`'s `attachments`
 * slot), and `AttachImageButton` is the explicit picker for the composer
 * footer. The encode pipeline itself lives in `../lib/imageAttachments.ts`;
 * this file only wires it to React state, mirroring how `MentionMenu.tsx`
 * wires `mentions.ts`.
 *
 * i18n keys referenced here (namespace `acp_chat`; see i18n.ts):
 *   attach_image_label / attach_image_tooltip / attachment_remove_label /
 *   image_attachment_fallback_name / image_not_supported_notice /
 *   image_too_large_notice / image_prepare_failed_notice
 */
import { Button, InlineNotice } from '@contenox/ui';
import { ImagePlus, X } from 'lucide-react';
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import {
  ImageAttachmentError,
  dataTransferHasImages,
  imageFilesFromDataTransfer,
  prepareImageAttachment,
  type ImageAttachmentErrorReason,
  type PendingImageAttachment,
} from '../lib/imageAttachments';

/** What the transient composer notice reports: a rejected capability, or a per-file failure reason. */
export type ImageAttachmentNotice = 'not_supported' | ImageAttachmentErrorReason;

/** How long a rejection notice stays up before clearing itself (ms). */
const NOTICE_TIMEOUT_MS = 5000;

export interface UseImageAttachmentsResult {
  images: PendingImageAttachment[];
  /** Transient rejection notice, or null — render via `ImageAttachmentNoticeView`. */
  notice: ImageAttachmentNotice | null;
  /** Encodes + stages picked/pasted/dropped files (rejections surface on `notice`). */
  addFiles: (files: File[]) => Promise<void>;
  removeImage: (id: string) => void;
  /** Empties the strip (on submit) and returns what was pending, so a failed submit can `restoreImages`. */
  takeImages: () => PendingImageAttachment[];
  restoreImages: (images: PendingImageAttachment[]) => void;
  /** Composer textarea `onPaste`: intercepts image payloads (stages them, or rejects when unsupported); plain-text pastes pass through untouched. */
  handlePaste: (e: ClipboardEvent<HTMLTextAreaElement>) => void;
  /** Composer wrapper `onDrop`: same interception for dropped image files. */
  handleDrop: (e: DragEvent<HTMLElement>) => void;
  /** Composer wrapper `onDragOver`: accepts the drag exactly when it carries images (required for drop to fire). */
  handleDragOver: (e: DragEvent<HTMLElement>) => void;
}

/**
 * Pending-image state for one composer. `supported` is the session agent's
 * advertised `PromptCapabilities.image` — when false there is NO attach
 * affordance (the caller hides `AttachImageButton`), and paste/drop of images
 * is swallowed with a `not_supported` notice instead of being staged.
 */
export function useImageAttachments(supported: boolean): UseImageAttachmentsResult {
  const [images, setImages] = useState<PendingImageAttachment[]>([]);
  const [notice, setNotice] = useState<ImageAttachmentNotice | null>(null);
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (noticeTimer.current) clearTimeout(noticeTimer.current);
    },
    [],
  );

  const showNotice = useCallback((next: ImageAttachmentNotice) => {
    setNotice(next);
    if (noticeTimer.current) clearTimeout(noticeTimer.current);
    noticeTimer.current = setTimeout(() => {
      noticeTimer.current = null;
      setNotice(null);
    }, NOTICE_TIMEOUT_MS);
  }, []);

  const addFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return;
      if (!supported) {
        showNotice('not_supported');
        return;
      }
      for (const file of files) {
        try {
          const attachment = await prepareImageAttachment(file);
          setImages(prev => [...prev, attachment]);
        } catch (err) {
          showNotice(err instanceof ImageAttachmentError ? err.reason : 'process_failed');
        }
      }
    },
    [supported, showNotice],
  );

  const removeImage = useCallback((id: string) => {
    setImages(prev => prev.filter(img => img.id !== id));
  }, []);

  // Read-then-clear via the state updater so a submit races nothing: whatever
  // the strip held at that instant is exactly what the turn carries.
  const imagesRef = useRef(images);
  imagesRef.current = images;
  const takeImages = useCallback(() => {
    const taken = imagesRef.current;
    setImages([]);
    return taken;
  }, []);

  const restoreImages = useCallback((restored: PendingImageAttachment[]) => {
    if (restored.length === 0) return;
    setImages(prev => [...restored, ...prev]);
  }, []);

  const handlePaste = useCallback(
    (e: ClipboardEvent<HTMLTextAreaElement>) => {
      const files = imageFilesFromDataTransfer(e.clipboardData);
      if (files.length === 0) return; // Plain text paste — default behavior.
      e.preventDefault();
      void addFiles(files);
    },
    [addFiles],
  );

  const handleDrop = useCallback(
    (e: DragEvent<HTMLElement>) => {
      const files = imageFilesFromDataTransfer(e.dataTransfer);
      if (files.length === 0) return;
      e.preventDefault();
      void addFiles(files);
    },
    [addFiles],
  );

  const handleDragOver = useCallback((e: DragEvent<HTMLElement>) => {
    // Accept the drag whenever it carries images — even when unsupported, so
    // the drop lands here and gets the explanatory notice instead of the
    // browser navigating to the dropped file.
    if (dataTransferHasImages(e.dataTransfer)) e.preventDefault();
  }, []);

  return {
    images,
    notice,
    addFiles,
    removeImage,
    takeImages,
    restoreImages,
    handlePaste,
    handleDrop,
    handleDragOver,
  };
}

/**
 * Removable thumbnail chips for the pending attachments, rendered inside the
 * composer shell via `ChatComposer`'s `attachments` slot. Renders null when
 * empty so the caller can pass it unconditionally.
 */
export function ComposerImageAttachments({
  images,
  onRemove,
}: {
  images: PendingImageAttachment[];
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (images.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-2" data-testid="composer-image-attachments">
      {images.map(img => {
        const name = img.name || t('acp_chat.image_attachment_fallback_name');
        return (
          <div key={img.id} className="relative" data-testid="composer-image-attachment">
            <img
              src={`data:${img.mimeType};base64,${img.data}`}
              alt={name}
              title={name}
              className="border-surface-300 dark:border-dark-surface-400 h-14 w-14 rounded border object-cover"
            />
            <button
              type="button"
              onClick={() => onRemove(img.id)}
              aria-label={t('acp_chat.attachment_remove_label', { name })}
              title={t('acp_chat.attachment_remove_label', { name })}
              className="bg-surface-50 text-text border-surface-300 hover:bg-surface-100 dark:bg-dark-surface-600 dark:text-dark-text dark:border-dark-surface-400 dark:hover:bg-dark-surface-500 absolute -top-1.5 -right-1.5 rounded-full border p-0.5 shadow-sm">
              <X className="h-3 w-3" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

/**
 * The explicit attach affordance for the composer footer: an icon button
 * driving a hidden `<input type="file" accept="image/*">`. The caller renders
 * it ONLY when the agent advertises image support (see `canAttachImages`).
 */
export function AttachImageButton({
  onFiles,
  disabled,
}: {
  onFiles: (files: File[]) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        data-testid="composer-image-input"
        onChange={e => {
          const files = Array.from(e.currentTarget.files ?? []);
          // Reset so picking the same file again re-fires onChange.
          e.currentTarget.value = '';
          if (files.length > 0) onFiles(files);
        }}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        disabled={disabled}
        aria-label={t('acp_chat.attach_image_label')}
        title={t('acp_chat.attach_image_tooltip')}
        onClick={() => inputRef.current?.click()}
        data-testid="composer-attach-image">
        <ImagePlus className="h-4 w-4" />
      </Button>
    </>
  );
}

/** The transient rejection notice above the composer — maps each `ImageAttachmentNotice` to its localized text. */
export function ImageAttachmentNoticeView({ notice }: { notice: ImageAttachmentNotice | null }) {
  const { t } = useTranslation();
  if (!notice) return null;
  const text =
    notice === 'not_supported'
      ? t('acp_chat.image_not_supported_notice')
      : notice === 'too_large'
        ? t('acp_chat.image_too_large_notice')
        : t('acp_chat.image_prepare_failed_notice');
  return (
    <InlineNotice
      variant="warning"
      className="mb-2 rounded-md border"
      data-testid="composer-image-notice">
      {text}
    </InlineNotice>
  );
}
