import { useEffect, useRef, useState } from 'react';
import { Bot, Loader2, Paperclip, Send } from 'lucide-react';
import { cn } from '../lib/cn';
import { formatTime } from '../lib/format';
import { Button } from './ui/Button';
import { Tooltip } from './ui/Tooltip';

export interface ConversationMessage {
  id: string;
  author_type: string;
  visibility?: string;
  body: string;
  created_at: string;
  /** True when the assistant drafted the message. It is labeled to the customer. */
  ai_generated?: boolean;
}

interface ConversationProps {
  messages: ConversationMessage[];
  perspective: 'customer' | 'merchant';
  onSend: (body: string) => Promise<void>;
  disabled?: boolean;
  emptyText?: string;
}

function authorKind(authorType: string): 'customer' | 'merchant' | 'system' {
  const value = (authorType || '').toUpperCase();
  if (value === 'CUSTOMER') return 'customer';
  if (value === 'MERCHANT_USER' || value === 'MERCHANT') return 'merchant';
  return 'system';
}

/** An AI-drafted message is labeled for the customer, whoever is reading it. */
function AiBadge() {
  return (
    <span className="inline-flex items-center gap-1 rounded-pill border-2 border-line bg-info px-1.5 py-0.5 text-[10px] font-extrabold uppercase tracking-[0.06em] text-[color:var(--info-ink)]">
      <Bot className="h-3 w-3" aria-hidden="true" strokeWidth={3} />
      AI-assisted
    </span>
  );
}

export function Conversation({
  messages,
  perspective,
  onSend,
  disabled = false,
  emptyText = 'No messages yet. Send the first one to start the conversation.',
}: ConversationProps) {
  const [draft, setDraft] = useState('');
  const [pending, setPending] = useState<ConversationMessage[]>([]);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const allMessages = [...messages, ...pending].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );

  useEffect(() => {
    const node = listRef.current;
    if (node) node.scrollTop = node.scrollHeight;
  }, [allMessages.length]);

  const resize = () => {
    const node = textareaRef.current;
    if (!node) return;
    node.style.height = 'auto';
    node.style.height = `${Math.min(node.scrollHeight, 128)}px`;
  };

  const send = async () => {
    const body = draft.trim();
    if (!body || sending || disabled) return;

    const optimistic: ConversationMessage = {
      id: `pending-${Date.now()}`,
      author_type: perspective === 'customer' ? 'CUSTOMER' : 'MERCHANT_USER',
      visibility: 'SHARED',
      body,
      created_at: new Date().toISOString(),
    };

    setPending((items) => [...items, optimistic]);
    setDraft('');
    setSending(true);
    setError(null);
    if (textareaRef.current) textareaRef.current.style.height = 'auto';

    try {
      await onSend(body);
    } catch {
      setDraft(body);
      setError('We could not send your message. Check your connection and try again.');
    } finally {
      setSending(false);
      setPending((items) => items.filter((item) => item.id !== optimistic.id));
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div
        ref={listRef}
        className="scrollbar-thin flex max-h-[520px] min-h-[140px] flex-col gap-4 overflow-y-auto pr-1"
        aria-live="polite"
        aria-label="Conversation"
      >
        {allMessages.length === 0 && (
          <p className="my-auto py-8 text-center text-body text-muted">{emptyText}</p>
        )}
        {allMessages.map((message) => {
          const kind = authorKind(message.author_type);
          const mine = kind === perspective;
          const aiAssisted = message.ai_generated === true;

          if (kind === 'system' && !aiAssisted) {
            return (
              <p
                key={message.id}
                className="animate-fade-in-fast self-center px-4 text-center text-meta italic text-muted"
              >
                {message.body}
              </p>
            );
          }

          return (
            <div
              key={message.id}
              className={cn(
                'flex max-w-[min(70ch,85%)] animate-fade-in-fast flex-col gap-1',
                mine ? 'items-end self-end' : 'items-start self-start',
              )}
            >
              <div
                className={cn(
                  'whitespace-pre-wrap break-words rounded border-2 border-line px-3 py-2 text-body shadow-hard',
                  mine
                    ? 'bg-[#003087] text-white'
                    : 'bg-surface-alt text-ink',
                )}
              >
                {message.body}
              </div>
              <p className="flex flex-wrap items-center gap-1.5 font-mono text-[11px] leading-4 text-muted">
                {aiAssisted ? (
                  <>
                    <AiBadge />
                    <span>Sent by {kind === 'merchant' ? 'the merchant' : 'FRACTAL'}</span>
                  </>
                ) : (
                  <span>{mine ? 'You' : kind === 'merchant' ? 'Merchant' : 'Customer'}</span>
                )}
                <span aria-hidden="true">·</span>
                <time dateTime={message.created_at}>{formatTime(message.created_at)}</time>
              </p>
            </div>
          );
        })}
      </div>

      <form
        className="mt-4 border-t border-line pt-3"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <div className="flex items-end gap-2 rounded border-2 border-line bg-surface p-1.5 transition-shadow focus-within:shadow-[4px_4px_0_var(--brand-sky)]">
          <label htmlFor="conversation-composer" className="sr-only">
            Write a message
          </label>
          <textarea
            id="conversation-composer"
            ref={textareaRef}
            rows={1}
            value={draft}
            disabled={disabled || sending}
            placeholder="Write a message…"
            onChange={(event) => {
              setDraft(event.target.value);
              resize();
            }}
            onKeyDown={(event) => {
              if (
                event.key === 'Enter' &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing
              ) {
                event.preventDefault();
                void send();
              }
            }}
            className="max-h-32 min-h-[40px] flex-1 resize-none bg-transparent px-2 py-2 text-body text-ink placeholder:text-muted/70 focus:outline-none disabled:cursor-not-allowed"
          />
          {/* TODO(backend): add an evidence-upload endpoint before enabling attachments. */}
          <Tooltip content="Attaching evidence isn’t available yet" side="top">
            <Button
              variant="ghost"
              size="sm"
              type="button"
              disabled
              aria-label="Attach evidence"
              className="h-10 w-10 px-0"
            >
              <Paperclip className="h-4 w-4" aria-hidden="true" />
            </Button>
          </Tooltip>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={!draft.trim() || disabled || sending}
            aria-label="Send message"
          >
            {sending ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Send className="h-4 w-4" aria-hidden="true" />
            )}
            Send
          </Button>
        </div>
        <div className="mt-1.5 flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted">
          <span>Enter to send, Shift + Enter for a new line</span>
          {error && (
            <span role="alert" className="font-bold text-[color:var(--danger-text)]">
              {error}
            </span>
          )}
        </div>
      </form>
    </div>
  );
}
