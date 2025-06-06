import { Button, Form, Spinner, Textarea } from '@contenox/ui';
import { t } from 'i18next';
import { FormEvent } from 'react';

type MessageInputFormProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit: (e: FormEvent) => void;
  placeholder?: string;
  isPending: boolean;
  buttonLabel?: string;
  title: string;
  className?: string;
};

export const MessageInputForm = ({
  value,
  onChange,
  onSubmit,
  placeholder = t('chat.input_placeholder'),
  isPending,
  buttonLabel = t('chat.send_button'),
  title,
  className,
}: MessageInputFormProps) => {
  return (
    <Form onSubmit={onSubmit} title={title} className={className}>
      <div className="flex gap-2">
        <Textarea
          placeholder={placeholder}
          value={value}
          onChange={e => onChange(e.target.value)}
          required
          className="flex-1"
        />
        <Button type="submit" variant="primary" disabled={isPending}>
          {isPending ? (
            <>
              <Spinner size="sm" className="mr-2" />
              {t('chat.sending_button')}
            </>
          ) : (
            buttonLabel
          )}
        </Button>
      </div>
    </Form>
  );
};
